package agent

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"mysql-agent/config"
	"mysql-agent/databases"
)

const (
	toolProcessList  = "mysql_processlist"
	toolInnoDBStatus = "mysql_innodb_status"
	toolGlobalStatus = "mysql_global_status"
	toolInnoDBTrx    = "mysql_innodb_trx"
	toolInnoDBMutex  = "mysql_innodb_mutex"
	toolSlowQueries  = "mysql_slow_queries"
	toolSchemaStats  = "mysql_schema_stats"
	toolConfigDiff   = "mysql_config_diff"
)

type ProcessListInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"description=返回的最大行数,minimum=1"`
}

type tableResult struct {
	Rows []map[string]string `json:"rows"`
}

type GlobalStatusInput struct {
	Keys []string `json:"keys,omitempty" jsonschema:"description=指定要返回的变量名列表"`
}

type InnoDBTrxInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"description=返回的最大行数,minimum=1"`
}

type SlowQueriesInput struct {
	Limit  int    `json:"limit,omitempty" jsonschema:"description=返回的最大行数,minimum=1"`
	Schema string `json:"schema,omitempty" jsonschema:"description=只返回指定数据库的结果"`
}

type SchemaStatsInput struct {
	Schema string `json:"schema,omitempty" jsonschema:"description=指定数据库名,默认为配置中的库"`
	Limit  int    `json:"limit,omitempty" jsonschema:"description=返回的最大表数量,minimum=1"`
}

type ConfigDiffInput struct {
	Variables []string `json:"variables,omitempty" jsonschema:"description=需要对比的运行时变量名"`
}

type ConfigDiffEntry struct {
	Parameter    string `json:"parameter"`
	ConfigValue  string `json:"config_value,omitempty"`
	RuntimeValue string `json:"runtime_value,omitempty"`
	Match        bool   `json:"match"`
}

type ConfigDiffResult struct {
	Items   []ConfigDiffEntry `json:"items"`
	Missing []string          `json:"missing,omitempty"`
}

type emptyInput struct{}

var (
	toolOnce sync.Once
	toolErr  error

	toolMap  map[string]tool.InvokableTool
	toolList []tool.InvokableTool
)

func ensureTools(ctx context.Context) ([]tool.InvokableTool, error) {
	toolOnce.Do(func() {
		toolMap = make(map[string]tool.InvokableTool)

		procTool, err := utils.InferTool(toolProcessList, "执行 `SHOW FULL PROCESSLIST`(必要时 `SHOW PROCESSLIST`) 以获取当前连接、状态与阻塞情况", processListTool)
		if err != nil {
			toolErr = fmt.Errorf("注册 processlist 工具失败: %w", err)
			return
		}
		toolMap[toolProcessList] = procTool
		toolList = append(toolList, procTool)
		log.Print("[ensureTools] registered mysql_processlist")

		innodbTool, err := utils.InferTool(toolInnoDBStatus, "执行 `SHOW ENGINE INNODB STATUS` 汇总锁等待、事务与缓冲区信息", innodbStatusTool)
		if err != nil {
			toolErr = fmt.Errorf("注册 innodb status 工具失败: %w", err)
			return
		}
		toolMap[toolInnoDBStatus] = innodbTool
		toolList = append(toolList, innodbTool)
		log.Print("[ensureTools] registered mysql_innodb_status")

		globalStatus, err := utils.InferTool(toolGlobalStatus, "执行 `SHOW GLOBAL STATUS` 返回 Threads_running、Connections 等指标，可按 keys 过滤", globalStatusTool)
		if err != nil {
			toolErr = fmt.Errorf("注册 global status 工具失败: %w", err)
			return
		}
		toolMap[toolGlobalStatus] = globalStatus
		toolList = append(toolList, globalStatus)
		log.Print("[ensureTools] registered mysql_global_status")

		innodbTrx, err := utils.InferTool(toolInnoDBTrx, "查询 `information_schema.innodb_trx`(可加 LIMIT) 查看长事务与等待信息", innodbTrxTool)
		if err != nil {
			toolErr = fmt.Errorf("注册 innodb trx 工具失败: %w", err)
			return
		}
		toolMap[toolInnoDBTrx] = innodbTrx
		toolList = append(toolList, innodbTrx)
		log.Print("[ensureTools] registered mysql_innodb_trx")

		innodbMutex, err := utils.InferTool(toolInnoDBMutex, "执行 `SHOW ENGINE INNODB MUTEX` 识别热点互斥锁", innodbMutexTool)
		if err != nil {
			toolErr = fmt.Errorf("注册 innodb mutex 工具失败: %w", err)
			return
		}
		toolMap[toolInnoDBMutex] = innodbMutex
		toolList = append(toolList, innodbMutex)
		log.Print("[ensureTools] registered mysql_innodb_mutex")

		slowQueries, err := utils.InferTool(toolSlowQueries, "统计 `performance_schema.events_statements_summary_by_digest` 中 TOP 慢 SQL (按 SUM_TIMER_WAIT 排序)", slowQueriesTool)
		if err != nil {
			toolErr = fmt.Errorf("注册 slow queries 工具失败: %w", err)
			return
		}
		toolMap[toolSlowQueries] = slowQueries
		toolList = append(toolList, slowQueries)
		log.Print("[ensureTools] registered mysql_slow_queries")

		schemaStats, err := utils.InferTool(toolSchemaStats, "查询 `information_schema.tables` 计算数据/索引大小及 TOTAL_LENGTH，可按 schema/limit", schemaStatsTool)
		if err != nil {
			toolErr = fmt.Errorf("注册 schema stats 工具失败: %w", err)
			return
		}
		toolMap[toolSchemaStats] = schemaStats
		toolList = append(toolList, schemaStats)
		log.Print("[ensureTools] registered mysql_schema_stats")

		configDiff, err := utils.InferTool(toolConfigDiff, "读取 `SHOW VARIABLES` 并与配置文件及连接池参数对比 (涵盖 character_set_server、collation_server、max_connections 等)", configDiffTool)
		if err != nil {
			toolErr = fmt.Errorf("注册 config diff 工具失败: %w", err)
			return
		}
		toolMap[toolConfigDiff] = configDiff
		toolList = append(toolList, configDiff)
		log.Print("[ensureTools] registered mysql_config_diff")
	})

	if toolErr != nil {
		return nil, toolErr
	}
	return toolList, nil
}

func processListTool(ctx context.Context, input *ProcessListInput) (*tableResult, error) {
	rows, err := databases.QueryProcessList(ctx)
	if err != nil {
		return nil, err
	}

	normalized := normalizeRows(rows)
	if input != nil && input.Limit > 0 && input.Limit < len(normalized) {
		normalized = normalized[:input.Limit]
	}

	return &tableResult{Rows: normalized}, nil
}

type innoDBStatusInput struct{}

type innoDBStatusOutput struct {
	Sections []map[string]string `json:"sections"`
}

func innodbStatusTool(ctx context.Context, _ *innoDBStatusInput) (*innoDBStatusOutput, error) {
	rows, err := databases.QueryInnoDBStatus(ctx)
	if err != nil {
		return nil, err
	}

	normalized := normalizeRows(rows)
	return &innoDBStatusOutput{Sections: normalized}, nil
}

func globalStatusTool(ctx context.Context, input *GlobalStatusInput) (*tableResult, error) {
	rows, err := databases.QueryGlobalStatus(ctx)
	if err != nil {
		return nil, err
	}

	normalized := normalizeRows(rows)
	filtering := input != nil && len(input.Keys) > 0
	if filtering {
		filters := make(map[string]struct{}, len(input.Keys))
		ordered := make([]string, 0, len(input.Keys))
		for _, key := range input.Keys {
			k := strings.ToLower(strings.TrimSpace(key))
			if k == "" {
				continue
			}
			if _, exists := filters[k]; !exists {
				ordered = append(ordered, k)
			}
			filters[k] = struct{}{}
		}

		filteredRows := make([]map[string]string, 0, len(ordered))
		for _, row := range normalized {
			name := strings.ToLower(row["variable_name"])
			if _, ok := filters[name]; ok {
				filteredRows = append(filteredRows, row)
			}
		}

		if len(ordered) > 0 {
			sort.SliceStable(filteredRows, func(i, j int) bool {
				return indexOf(ordered, strings.ToLower(filteredRows[i]["variable_name"])) < indexOf(ordered, strings.ToLower(filteredRows[j]["variable_name"]))
			})
		}

		normalized = filteredRows
	}

	if !filtering && len(normalized) > 1 {
		sort.Slice(normalized, func(i, j int) bool {
			return normalized[i]["variable_name"] < normalized[j]["variable_name"]
		})
	}

	return &tableResult{Rows: normalized}, nil
}

func innodbTrxTool(ctx context.Context, input *InnoDBTrxInput) (*tableResult, error) {
	limit := 0
	if input != nil && input.Limit > 0 {
		limit = input.Limit
	}

	rows, err := databases.QueryInnoDBTrx(ctx, limit)
	if err != nil {
		return nil, err
	}

	normalized := normalizeRows(rows)
	return &tableResult{Rows: normalized}, nil
}

func innodbMutexTool(ctx context.Context, _ *emptyInput) (*tableResult, error) {
	rows, err := databases.QueryInnoDBMutex(ctx)
	if err != nil {
		return nil, err
	}

	normalized := normalizeRows(rows)
	return &tableResult{Rows: normalized}, nil
}

func slowQueriesTool(ctx context.Context, input *SlowQueriesInput) (*tableResult, error) {
	limit := 0
	if input != nil && input.Limit > 0 {
		limit = input.Limit
	}

	rows, err := databases.QuerySlowQueries(ctx, limit)
	if err != nil {
		return nil, err
	}

	normalized := normalizeRows(rows)
	if input != nil && strings.TrimSpace(input.Schema) != "" {
		target := strings.ToLower(strings.TrimSpace(input.Schema))
		filtered := normalized[:0]
		for _, row := range normalized {
			if strings.EqualFold(row["schema_name"], target) {
				filtered = append(filtered, row)
			}
		}
		normalized = filtered
	}

	return &tableResult{Rows: normalized}, nil
}

func schemaStatsTool(ctx context.Context, input *SchemaStatsInput) (*tableResult, error) {
	schema := ""
	limit := 0
	if input != nil {
		schema = input.Schema
		if input.Limit > 0 {
			limit = input.Limit
		}
	}

	rows, err := databases.QuerySchemaStats(ctx, schema, limit)
	if err != nil {
		return nil, err
	}

	normalized := normalizeRows(rows)
	return &tableResult{Rows: normalized}, nil
}

func configDiffTool(ctx context.Context, input *ConfigDiffInput) (*ConfigDiffResult, error) {
	vars, err := databases.QueryGlobalVariables(ctx)
	if err != nil {
		return nil, err
	}

	requested := inputVariables(input)
	items := make([]ConfigDiffEntry, 0, len(requested))
	missing := make([]string, 0)

	for _, name := range requested {
		key := strings.ToLower(name)
		runtime := vars[key]
		if runtime == "" {
			missing = append(missing, name)
		}
		configVal, hasConfig := configValueFor(key)
		match := hasConfig && runtime != "" && strings.EqualFold(runtime, configVal)
		items = append(items, ConfigDiffEntry{
			Parameter:    name,
			ConfigValue:  configVal,
			RuntimeValue: runtime,
			Match:        match,
		})
	}

	poolEntries := poolDiffEntries()
	if len(poolEntries) > 0 {
		items = append(items, poolEntries...)
	}

	return &ConfigDiffResult{Items: items, Missing: missing}, nil
}

func inputVariables(input *ConfigDiffInput) []string {
	if input != nil && len(input.Variables) > 0 {
		cleaned := make([]string, 0, len(input.Variables))
		seen := make(map[string]struct{})
		for _, raw := range input.Variables {
			v := strings.TrimSpace(raw)
			if v == "" {
				continue
			}
			lower := strings.ToLower(v)
			if _, ok := seen[lower]; ok {
				continue
			}
			seen[lower] = struct{}{}
			cleaned = append(cleaned, v)
		}
		if len(cleaned) > 0 {
			return cleaned
		}
	}
	return []string{"character_set_server", "port"}
}

func configValueFor(name string) (string, bool) {
	cfg := config.AppConfig
	switch name {
	case "character_set_server", "character_set_database":
		return cfg.Database.Charset, true
	case "port":
		return strconv.Itoa(cfg.Database.Port), true
	case "host", "hostname":
		return cfg.Database.Host, true
	default:
		return "", false
	}
}

func poolDiffEntries() []ConfigDiffEntry {
	db, err := databases.GetDB()
	if err != nil {
		return nil
	}

	cfg := config.AppConfig
	stats := db.Stats()

	entries := []ConfigDiffEntry{
		{
			Parameter:    "connection_pool.max_open_conns",
			ConfigValue:  strconv.Itoa(cfg.Database.MaxOpenConns),
			RuntimeValue: strconv.Itoa(stats.MaxOpenConnections),
			Match:        cfg.Database.MaxOpenConns == stats.MaxOpenConnections,
		},
	}

	if cfg.Database.MaxIdleConns > 0 {
		entries = append(entries, ConfigDiffEntry{
			Parameter:    "connection_pool.max_idle_conns",
			ConfigValue:  strconv.Itoa(cfg.Database.MaxIdleConns),
			RuntimeValue: strconv.Itoa(cfg.Database.MaxIdleConns),
			Match:        true,
		})
	}

	if cfg.Database.ConnMaxLifetime > 0 {
		entries = append(entries, ConfigDiffEntry{
			Parameter:    "connection_pool.conn_max_lifetime",
			ConfigValue:  cfg.Database.ConnMaxLifetime.String(),
			RuntimeValue: cfg.Database.ConnMaxLifetime.String(),
			Match:        true,
		})
	}

	return entries
}

func indexOf(list []string, target string) int {
	for idx, val := range list {
		if val == target {
			return idx
		}
	}
	return len(list)
}

func normalizeRows(rows []map[string]any) []map[string]string {
	result := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		normalized := make(map[string]string, len(row))
		for k, v := range row {
			normalized[strings.ToLower(k)] = fmt.Sprintf("%v", v)
		}
		result = append(result, normalized)
	}
	return result
}

func RegisteredTools(ctx context.Context) ([]tool.InvokableTool, error) {
	return ensureTools(ctx)
}

type ToolDescriptor struct {
	Name string `json:"name"`
	Desc string `json:"description"`
}

func ToolDescriptors(ctx context.Context) ([]ToolDescriptor, error) {
	tools, err := ensureTools(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]ToolDescriptor, 0, len(tools))
	for _, tl := range tools {
		info, err := tl.Info(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, ToolDescriptor{Name: info.Name, Desc: info.Desc})
	}
	return result, nil
}

func CallTool(ctx context.Context, name string, rawArgs string) (string, error) {
	_, err := ensureTools(ctx)
	if err != nil {
		return "", err
	}

	tl, ok := toolMap[name]
	if !ok {
		return "", fmt.Errorf("未找到工具: %s", name)
	}

	args := strings.TrimSpace(rawArgs)
	if args == "" {
		args = "{}"
	}

	log.Printf("[CallTool] name=%s args=%s", name, truncate(args))

	output, err := tl.InvokableRun(ctx, args)
	if err != nil {
		return "", err
	}
	log.Printf("[CallTool] name=%s output=%s", name, truncate(output))
	return output, nil
}

func ToolNames(ctx context.Context) ([]string, error) {
	tools, err := ensureTools(ctx)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(tools))
	for _, tl := range tools {
		info, err := tl.Info(ctx)
		if err != nil {
			return nil, err
		}
		names = append(names, info.Name)
	}
	return names, nil
}
