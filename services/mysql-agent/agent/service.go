package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"mysql-agent/deepseek"
	"mysql-agent/mcp"
)

type QueryRequest struct {
	Query string `json:"query"`
}

type QueryResponse struct {
	Answer  string                 `json:"answer"`
	Sources []ToolSource           `json:"sources"`
	Raw     map[string]interface{} `json:"raw"`
}

type ToolSource struct {
	Tool        string                 `json:"tool"`
	Description string                 `json:"description,omitempty"`
	Status      string                 `json:"status"`
	Params      map[string]interface{} `json:"params,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

type Service struct {
	deepseekClient *deepseek.Client
	toolRegistry   *mcp.ToolRegistry
}

type toolPlan struct {
	Steps []toolPlanStep `json:"steps"`
}

type toolPlanStep struct {
	Tool   string                 `json:"tool"`
	Reason string                 `json:"reason"`
	Params map[string]interface{} `json:"params"`
}

type toolExecutionResult struct {
	Step        toolPlanStep
	Description string
	Output      interface{}
	Err         error
}

type summaryToolResult struct {
	Tool   string                 `json:"tool"`
	Params map[string]interface{} `json:"params"`
	Status string                 `json:"status"`
	Result interface{}            `json:"result,omitempty"`
	Error  string                 `json:"error,omitempty"`
}

type signalStatus struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Tool   string `json:"tool"`
	Status string `json:"status"`
	Notes  string `json:"notes,omitempty"`
}

const plannerSystemPrompt = `你是MySQL数据库诊断调度助手。请根据用户的数据库问题制定一个严格的工具执行计划。
仅可使用提供的工具名称。输出JSON对象 {"steps":[{"tool":"名称","reason":"为什么执行此工具","params":{}}]}。
如果无需工具，请输出 {"steps":[]}。不得输出解释性文本。`

const summarySystemPrompt = `只基于提供的 tool_results JSON 输出。不得编造任何未出现在 JSON 里的指标或结论。
输出为简明 DBA 报告（上限 250 行以内），包含以下结构：

结论摘要（TL;DR）：一句话健康结论 + 关键数值（带单位）。
关键指标表（Markdown 表格）：QPS/TPS、Threads_running、Threads_connected、Max_used_connections、Seconds_Behind_Master（如有）、活跃会话数等。
异常与风险：锁等待、复制延迟、长事务、Top 慢 SQL 指纹（fingerprint+avg_latency_ms+count）。
优先级行动项（P0/P1/P2）：每条 ≤ 1 行，含“为何+怎么做”。
来源：列出使用的工具与关键参数，例如 mysql.qps{instance_id=...,window=3600}。
缺失的数据以 “N/A” 标注。数字请标单位；时间用 ISO8601；不要写“看起来/可能/估计”之类的模糊词。

我会额外提供 required_signals 数组，里面列出诊断必须覆盖的指标与其工具及采集状态：
- 当 status 为 collected 时可正常引用该指标；
- 当 status 为 error 或 not_collected 或 unsupported 时，必须在报告中标注 N/A，且不可写出“无xxx”之类的否定结论，需明确指出数据缺失并引用 required_signals.

在输出前自检：
1. 所有数字均来自 tool_results；
2. 每条结论可对应至少一个工具来源；
3. 行动项数量 ≤ 5 条且与异常直接相关；
4. 总行数不超过 250。
若未满足，请在输出前自行纠正。`

var defaultToolSequence = []toolPlanStep{
	{Tool: "show_status", Reason: "获取核心状态指标", Params: map[string]interface{}{}},
	{Tool: "show_connections", Reason: "检查连接与会话情况", Params: map[string]interface{}{}},
	{Tool: "show_processlist", Reason: "查看活跃会话与长事务", Params: map[string]interface{}{"full": true}},
	{Tool: "slow_query_analysis", Reason: "审阅Top慢查询指纹", Params: map[string]interface{}{"limit": 10}},
}

var requiredSignalConfig = []struct {
	Key  string
	Name string
	Tool string
}{
	{Key: "slow_queries", Name: "慢查询情况", Tool: "slow_query_analysis"},
	{Key: "lock_waits", Name: "锁等待", Tool: "innodb_lock_waits"},
	{Key: "replication_delay", Name: "复制延迟", Tool: "replication_status"},
	{Key: "long_transactions", Name: "长事务", Tool: "innodb_trx"},
}

func NewService() (*Service, error) {
	client := deepseek.NewClient()

	toolRegistry, err := mcp.InitializeTools()
	if err != nil {
		return nil, fmt.Errorf("initialize tools: %w", err)
	}

	return &Service{
		deepseekClient: client,
		toolRegistry:   toolRegistry,
	}, nil
}

func (s *Service) Query(req QueryRequest, resp *QueryResponse) error {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		resp.Answer = "请输入有效的查询问题"
		return nil
	}

	log.Printf("[Agent] request query=\"%s\"", summarizeQuery(query))

	toolDefs := s.toolRegistry.GetToolDefinitions()
	plan, err := s.generateToolPlan(query, toolDefs)
	if err != nil {
		log.Printf("[Agent] plan_llm_failed err=%v fallback=default", err)
		plan = s.defaultPlan(toolDefs)
	}

	if len(plan.Steps) == 0 {
		log.Printf("[Agent] plan_empty fallback=default")
		plan = s.defaultPlan(toolDefs)
	}

	executions := s.executePlan(plan)
	resp.Sources = buildSources(executions)
	resp.Raw = buildRaw(executions)

	answer, summaryErr := s.generateDBASummary(query, plan, executions, toolDefs)
	if summaryErr != nil {
		log.Printf("[Agent] summary_failed err=%v returning=fallback", summaryErr)
		resp.Answer = s.buildFallbackAnswer(executions)
		return nil
	}

	resp.Answer = answer
	return nil
}

func (s *Service) generateToolPlan(question string, toolDefs []mcp.ToolDefinition) (toolPlan, error) {
	if s.deepseekClient == nil {
		return toolPlan{}, fmt.Errorf("deepseek client not initialised")
	}

	start := time.Now()
	log.Printf("[Agent] plan_llm_start tools=%d", len(toolDefs))

	payload := struct {
		Question string `json:"question"`
		Tools    []struct {
			Name        string      `json:"name"`
			Description string      `json:"description"`
			Parameters  interface{} `json:"parameters"`
		} `json:"tools"`
	}{Question: question}

	for _, def := range toolDefs {
		payload.Tools = append(payload.Tools, struct {
			Name        string      `json:"name"`
			Description string      `json:"description"`
			Parameters  interface{} `json:"parameters"`
		}{
			Name:        def.Function.Name,
			Description: def.Function.Description,
			Parameters:  def.Function.Parameters,
		})
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return toolPlan{}, fmt.Errorf("marshal plan payload: %w", err)
	}

	userPrompt := fmt.Sprintf("请根据以下输入输出JSON计划：\n```json\n%s\n```", string(payloadJSON))

	reqBody := deepseek.ChatRequest{
		Model: s.deepseekClient.Model,
		Messages: []deepseek.Message{
			{Role: "system", Content: plannerSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return toolPlan{}, fmt.Errorf("marshal planner request: %w", err)
	}

	chatResp, err := s.deepseekClient.ChatWithBody(jsonData)
	if err != nil {
		return toolPlan{}, fmt.Errorf("call DeepSeek planner: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return toolPlan{}, fmt.Errorf("planner returned empty choices")
	}

	content := cleanJSONBlock(chatResp.Choices[0].Message.Content)
	var plan toolPlan
	if err := json.Unmarshal([]byte(content), &plan); err != nil {
		return toolPlan{}, fmt.Errorf("parse plan JSON: %w", err)
	}

	normalizePlan(&plan)
	log.Printf("[Agent] plan_ready steps=%d duration=%s", len(plan.Steps), time.Since(start))
	return plan, nil
}

func (s *Service) defaultPlan(toolDefs []mcp.ToolDefinition) toolPlan {
	available := map[string]mcp.ToolDefinition{}
	for _, def := range toolDefs {
		available[def.Function.Name] = def
	}

	var steps []toolPlanStep
	for _, step := range defaultToolSequence {
		if _, ok := available[step.Tool]; ok {
			params := map[string]interface{}{}
			for k, v := range step.Params {
				params[k] = v
			}
			steps = append(steps, toolPlanStep{
				Tool:   step.Tool,
				Reason: step.Reason,
				Params: params,
			})
		}
	}

	plan := toolPlan{Steps: steps}
	normalizePlan(&plan)
	log.Printf("[Agent] plan_default steps=%d", len(plan.Steps))
	return plan
}

func (s *Service) executePlan(plan toolPlan) []toolExecutionResult {
	var results []toolExecutionResult
	for _, step := range plan.Steps {
		tool, exists := s.toolRegistry.GetTool(step.Tool)
		if !exists {
			log.Printf("[ToolExec] missing tool=%s", step.Tool)
			results = append(results, toolExecutionResult{
				Step: step,
				Err:  fmt.Errorf("tool %s not registered", step.Tool),
			})
			continue
		}

		def := tool.GetDefinition()
		execStart := time.Now()
		output, err := tool.Execute(step.Params)
		duration := time.Since(execStart)
		if err != nil {
			log.Printf("[ToolExec] tool=%s status=error duration=%s err=%v", step.Tool, duration, err)
		} else {
			log.Printf("[ToolExec] tool=%s status=ok duration=%s", step.Tool, duration)
		}

		results = append(results, toolExecutionResult{
			Step:        step,
			Description: def.Function.Description,
			Output:      output,
			Err:         err,
		})
	}

	return results
}

func (s *Service) generateDBASummary(question string, plan toolPlan, executions []toolExecutionResult, toolDefs []mcp.ToolDefinition) (string, error) {
	if s.deepseekClient == nil {
		return "", fmt.Errorf("deepseek client not initialised")
	}

	start := time.Now()
	log.Printf("[Agent] summary_llm_start tools=%d signals=%d", len(executions), len(requiredSignalConfig))

	var toolResults []summaryToolResult
	for _, exec := range executions {
		status := "success"
		errMsg := ""
		if exec.Err != nil {
			status = "error"
			errMsg = exec.Err.Error()
		}

		params := map[string]interface{}{}
		if exec.Step.Params != nil {
			for k, v := range exec.Step.Params {
				params[k] = v
			}
		}

		toolResults = append(toolResults, summaryToolResult{
			Tool:   exec.Step.Tool,
			Params: params,
			Status: status,
			Result: exec.Output,
			Error:  errMsg,
		})
	}

	requiredSignals := buildSignalStatuses(toolDefs, executions)

	payload := struct {
		Question    string              `json:"question"`
		Plan        []toolPlanStep      `json:"plan"`
		ToolResults []summaryToolResult `json:"tool_results"`
		Signals     []signalStatus      `json:"required_signals"`
	}{
		Question:    question,
		Plan:        plan.Steps,
		ToolResults: toolResults,
		Signals:     requiredSignals,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal summary payload: %w", err)
	}

	userPrompt := fmt.Sprintf("以下是分析输入，请严格仅基于其中的数据生成报告：\n```json\n%s\n```", string(payloadJSON))

	reqBody := deepseek.ChatRequest{
		Model: s.deepseekClient.Model,
		Messages: []deepseek.Message{
			{Role: "system", Content: summarySystemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal summary request: %w", err)
	}

	chatResp, err := s.deepseekClient.ChatWithBody(jsonData)
	if err != nil {
		return "", fmt.Errorf("call DeepSeek summary: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("summary returned empty choices")
	}

	log.Printf("[Agent] summary_ready duration=%s", time.Since(start))
	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}

func (s *Service) buildFallbackAnswer(executions []toolExecutionResult) string {
	snapshot := map[string]interface{}{}
	for _, exec := range executions {
		entry := map[string]interface{}{
			"params": exec.Step.Params,
		}
		if exec.Err != nil {
			entry["error"] = exec.Err.Error()
		} else {
			entry["result"] = exec.Output
		}

		key := exec.Step.Tool
		if existing, ok := snapshot[key]; ok {
			list := existing.([]map[string]interface{})
			list = append(list, entry)
			snapshot[key] = list
		} else {
			snapshot[key] = []map[string]interface{}{entry}
		}
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "工具执行完成，但无法生成总结，参考原始结果"
	}

	return fmt.Sprintf("工具执行完成，但总结阶段失败，请参考原始结果：\n```json\n%s\n```", string(data))
}

func buildSources(executions []toolExecutionResult) []ToolSource {
	var sources []ToolSource
	for _, exec := range executions {
		status := "success"
		errMsg := ""
		if exec.Err != nil {
			status = "error"
			errMsg = exec.Err.Error()
		}

		params := map[string]interface{}{}
		if exec.Step.Params != nil {
			for k, v := range exec.Step.Params {
				params[k] = v
			}
		}

		sources = append(sources, ToolSource{
			Tool:        exec.Step.Tool,
			Description: exec.Description,
			Status:      status,
			Params:      params,
			Error:       errMsg,
		})
	}
	return sources
}

func buildRaw(executions []toolExecutionResult) map[string]interface{} {
	raw := make(map[string]interface{})
	for _, exec := range executions {
		entry := map[string]interface{}{
			"params": exec.Step.Params,
		}
		if exec.Err != nil {
			entry["error"] = exec.Err.Error()
		} else {
			entry["result"] = exec.Output
		}

		toolName := exec.Step.Tool
		if existing, ok := raw[toolName]; ok {
			list := existing.([]map[string]interface{})
			list = append(list, entry)
			raw[toolName] = list
		} else {
			raw[toolName] = []map[string]interface{}{entry}
		}
	}
	return raw
}

func buildSignalStatuses(toolDefs []mcp.ToolDefinition, executions []toolExecutionResult) []signalStatus {
	available := make(map[string]bool)
	for _, def := range toolDefs {
		available[def.Function.Name] = true
	}

	execStatus := make(map[string]toolExecutionResult)
	for _, exec := range executions {
		execStatus[exec.Step.Tool] = exec
	}

	var signals []signalStatus
	for _, cfg := range requiredSignalConfig {
		status := "not_collected"
		notes := ""

		if !available[cfg.Tool] {
			status = "unsupported"
			notes = "tool not registered"
		} else if exec, ok := execStatus[cfg.Tool]; ok {
			if exec.Err != nil {
				status = "error"
				notes = exec.Err.Error()
			} else {
				status = "collected"
			}
		}

		signals = append(signals, signalStatus{
			Key:    cfg.Key,
			Name:   cfg.Name,
			Tool:   cfg.Tool,
			Status: status,
			Notes:  notes,
		})
	}

	return signals
}

func cleanJSONBlock(input string) string {
	trimmed := strings.TrimSpace(input)
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```json\n")
		trimmed = strings.TrimPrefix(trimmed, "```JSON\n")
		trimmed = strings.TrimPrefix(trimmed, "```json")
		trimmed = strings.TrimPrefix(trimmed, "```JSON")
		trimmed = strings.TrimPrefix(trimmed, "```")
		trimmed = strings.TrimPrefix(trimmed, "\n")
		if idx := strings.LastIndex(trimmed, "```"); idx >= 0 {
			trimmed = trimmed[:idx]
		}
	}
	return strings.TrimSpace(trimmed)
}

func normalizePlan(plan *toolPlan) {
	var normalised []toolPlanStep
	seen := make(map[string]bool)
	for _, step := range plan.Steps {
		name := strings.TrimSpace(step.Tool)
		if name == "" {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true

		params := map[string]interface{}{}
		if step.Params != nil {
			for k, v := range step.Params {
				params[k] = v
			}
		}

		normalised = append(normalised, toolPlanStep{
			Tool:   name,
			Reason: strings.TrimSpace(step.Reason),
			Params: params,
		})
	}
	plan.Steps = normalised
}

func summarizeQuery(q string) string {
	const max = 80
	q = strings.ReplaceAll(q, "\n", " ")
	q = strings.TrimSpace(q)
	r := []rune(q)
	if len(r) <= max {
		return q
	}
	return string(r[:max-3]) + "..."
}
