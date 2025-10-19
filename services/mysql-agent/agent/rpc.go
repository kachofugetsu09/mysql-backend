package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
)

type ToolCallSpec struct {
	Name   string          `json:"name"`
	Args   json.RawMessage `json:"args,omitempty"`
	Reason string          `json:"reason,omitempty"`
}

type QueryRequest struct {
	Query          string            `json:"query"`
	Tools          []ToolCallSpec    `json:"tools,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
	Context        map[string]string `json:"context,omitempty"`
}

type ToolRun struct {
	Name       string      `json:"name"`
	Reason     string      `json:"reason,omitempty"`
	Input      interface{} `json:"input,omitempty"`
	Output     interface{} `json:"output,omitempty"`
	Error      string      `json:"error,omitempty"`
	DurationMs int64       `json:"duration_ms"`
}

type AnalysisResult struct {
	Summary string `json:"summary,omitempty"`
	Error   string `json:"error,omitempty"`
}

type QueryResponse struct {
	Analysis AnalysisResult         `json:"analysis"`
	ToolRuns []ToolRun              `json:"tool_runs"`
	Raw      map[string]interface{} `json:"raw,omitempty"`
}

type RPCService struct{}

const defaultQueryTimeout = 60 * time.Second

func (RPCService) Query(req QueryRequest, resp *QueryResponse) error {
	if strings.TrimSpace(req.Query) == "" {
		return fmt.Errorf("query 不能为空")
	}

	timeout := defaultQueryTimeout
	if req.TimeoutSeconds > 0 {
		timeout = time.Duration(req.TimeoutSeconds) * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	plan := req.Tools
	if len(plan) == 0 {
		var refusal string
		var err error
		plan, refusal, err = planWithLLM(ctx, req)
		if err != nil {
			log.Printf("[Query] planWithLLM error: %v", err)
			resp.Analysis.Error = fmt.Sprintf("规划工具失败: %v", err)
			return nil
		}
		if refusal != "" {
			log.Printf("[Query] planWithLLM refusal: %s", refusal)
			resp.Analysis.Error = refusal
			return nil
		}
	}

	if len(plan) == 0 {
		resp.Analysis.Error = "无可用工具执行该请求"
		return nil
	}

	log.Printf("[Query] query=%q plan=%v", req.Query, summarizePlan(plan))

	toolRuns := make([]ToolRun, 0, len(plan))
	toolOutputs := make([]map[string]interface{}, 0, len(plan))
	failure := ""

	for _, spec := range plan {
		argsStr := string(spec.Args)
		if strings.TrimSpace(spec.Reason) != "" {
			log.Printf("[Query] invoking tool=%s reason=%s", spec.Name, spec.Reason)
		} else {
			log.Printf("[Query] invoking tool=%s", spec.Name)
		}
		start := time.Now()
		outputStr, err := CallTool(ctx, spec.Name, argsStr)
		duration := time.Since(start).Milliseconds()

		run := ToolRun{Name: spec.Name, Reason: spec.Reason, Input: safeParseJSON(argsStr), DurationMs: duration}
		if err != nil {
			run.Error = err.Error()
			failure = fmt.Sprintf("工具 %s 执行失败: %v", spec.Name, err)
			toolRuns = append(toolRuns, run)
			log.Printf("[Query] tool=%s failed: %v", spec.Name, err)
			break
		}

		parsed := safeParseJSON(outputStr)
		run.Output = parsed
		toolRuns = append(toolRuns, run)
		toolOutputs = append(toolOutputs, map[string]interface{}{
			"name":   spec.Name,
			"output": parsed,
		})
	}

	resp.ToolRuns = toolRuns
	resp.Raw = map[string]interface{}{
		"tool_outputs": toolOutputs,
	}

	if failure != "" {
		resp.Analysis.Error = failure
		return nil
	}

	analysis, err := analyzeWithLLM(ctx, req.Query, toolOutputs)
	if err != nil {
		log.Printf("[Query] analyzeWithLLM failed: %v", err)
		resp.Analysis.Error = err.Error()
		resp.Raw["llm_error"] = err.Error()
		return nil
	}

	log.Print("[Query] analyzeWithLLM success")
	resp.Analysis.Summary = analysis.Content
	if analysis.ResponseMeta != nil {
		resp.Raw["response_meta"] = analysis.ResponseMeta
	}
	return nil
}

func analyzeWithLLM(ctx context.Context, query string, toolOutputs []map[string]interface{}) (*schema.Message, error) {
	log.Print("[analyzeWithLLM] start")
	messages := []*schema.Message{
		{
			Role:    schema.System,
			Content: "你是 MySQL 运维诊断助手，会根据工具返回的数据给出结论和建议。",
		},
		{
			Role:    schema.User,
			Content: fmt.Sprintf("用户问题：%s", query),
		},
	}

	for _, item := range toolOutputs {
		name, _ := item["name"].(string)
		pretty, _ := json.MarshalIndent(item["output"], "", "  ")
		messages = append(messages, &schema.Message{
			Role:    schema.System,
			Content: fmt.Sprintf("工具 %s 输出:\n%s", name, string(pretty)),
		})
	}

	messages = append(messages, &schema.Message{
		Role:    schema.User,
		Content: "请结合以上工具数据给出诊断以及后续建议，结构化输出结论和建议。",
	})

	result, err := Generate(ctx, messages)
	if err != nil {
		log.Printf("[analyzeWithLLM] Generate error: %v", err)
		return nil, fmt.Errorf("LLM 分析失败: %w", err)
	}
	if result == nil {
		log.Print("[analyzeWithLLM] empty response")
		return nil, fmt.Errorf("LLM 返回为空")
	}
	log.Print("[analyzeWithLLM] success")
	return result, nil
}

func RegisterRPC(server RPCRegistrar) error {
	return server.RegisterName("Agent", RPCService{})
}

type RPCRegistrar interface {
	RegisterName(name string, rcvr interface{}) error
}

func safeParseJSON(raw string) interface{} {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" || trimmed == "null" {
		return nil
	}
	var v interface{}
	if err := json.Unmarshal([]byte(trimmed), &v); err != nil {
		return trimmed
	}
	return v
}

func summarizePlan(plan []ToolCallSpec) []string {
	names := make([]string, 0, len(plan))
	for _, p := range plan {
		if strings.TrimSpace(p.Reason) != "" {
			names = append(names, fmt.Sprintf("%s(%s)", p.Name, p.Reason))
		} else {
			names = append(names, p.Name)
		}
	}
	return names
}

type llmPlanResponse struct {
	CanAnswer bool             `json:"can_answer"`
	Reason    string           `json:"reason,omitempty"`
	Tools     []plannedToolCmd `json:"tools"`
}

type plannedToolCmd struct {
	Name   string                 `json:"name"`
	Args   map[string]interface{} `json:"args,omitempty"`
	Reason string                 `json:"reason,omitempty"`
}

func planWithLLM(ctx context.Context, req QueryRequest) ([]ToolCallSpec, string, error) {
	descriptors, err := ToolDescriptors(ctx)
	if err != nil {
		return nil, "", err
	}

	prompt := buildPlannerPrompt(descriptors, req.Query)
	log.Printf("[planWithLLM] prompt=%s", truncate(prompt))

	messages := []*schema.Message{
		{Role: schema.System, Content: "你是一个数据库诊断工具调度助手，会根据用户需求在允许的工具中规划执行步骤。"},
		{Role: schema.User, Content: prompt},
	}

	result, err := Generate(ctx, messages)
	if err != nil {
		return nil, "", fmt.Errorf("请求 LLM 规划失败: %w", err)
	}

	raw := result.Content
	log.Printf("[planWithLLM] raw_response=%s", truncate(raw))

	planResp, err := parsePlanJSON(raw)
	if err != nil {
		return nil, "", err
	}

	if !planResp.CanAnswer {
		if planResp.Reason != "" {
			return nil, fmt.Sprintf("无法处理请求: %s", planResp.Reason), nil
		}
		return nil, "请求超出工具能力范围", nil
	}

	tools := make([]ToolCallSpec, 0, len(planResp.Tools))
	for _, t := range planResp.Tools {
		if strings.TrimSpace(t.Name) == "" {
			continue
		}
		var rawArgs json.RawMessage
		if t.Args != nil {
			bytes, err := json.Marshal(t.Args)
			if err != nil {
				return nil, "", fmt.Errorf("序列化工具参数失败: %w", err)
			}
			rawArgs = bytes
		}
		tools = append(tools, ToolCallSpec{Name: t.Name, Args: rawArgs, Reason: t.Reason})
	}

	return tools, "", nil
}

func buildPlannerPrompt(descriptors []ToolDescriptor, query string) string {
	var sb strings.Builder
	sb.WriteString("可用工具如下 (仅能从中选择):\n")
	for _, d := range descriptors {
		sb.WriteString("- ")
		sb.WriteString(d.Name)
		sb.WriteString(": ")
		sb.WriteString(d.Desc)
		sb.WriteString("\n")
	}
	sb.WriteString("\n请根据用户问题决定是否可以通过这些工具解决。如果不能解决，输出 JSON: {\"can_answer\": false, \"reason\": \"原因\"}。" +
		"如果可以，输出 JSON: {\"can_answer\": true, \"tools\": [{\"name\": 工具名, \"args\": 参数对象, \"reason\": \"调用原因\"}] }。" +
		"调用原因需简要说明此工具如何辅助回答。参数对象可以为空对象或包含必要字段，禁止使用未提供的工具。" +
		"用户问题: ")
	sb.WriteString(query)
	return sb.String()
}

func parsePlanJSON(raw string) (llmPlanResponse, error) {
	var plan llmPlanResponse
	raw = strings.TrimSpace(raw)
	raw = stripMarkdownFence(raw)
	if idx := strings.Index(raw, "{"); idx > 0 {
		raw = raw[idx:]
	}
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return plan, fmt.Errorf("解析 LLM 规划响应失败: %w", err)
	}
	return plan, nil
}

func truncate(s string) string {
	const limit = 256
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}

func stripMarkdownFence(raw string) string {
	start := strings.Index(raw, "```")
	if start == -1 {
		return raw
	}
	end := strings.LastIndex(raw, "```")
	if end == -1 || end <= start+3 {
		segment := raw[start+3:]
		return strings.TrimSpace(segment)
	}
	segment := raw[start+3 : end]
	segment = strings.TrimSpace(segment)
	if idx := strings.Index(segment, "\n"); idx >= 0 {
		firstLine := strings.TrimSpace(segment[:idx])
		if len(firstLine) > 0 && !strings.HasPrefix(firstLine, "{") && !strings.HasPrefix(firstLine, "[") {
			segment = strings.TrimSpace(segment[idx+1:])
		}
	}
	return segment
}
