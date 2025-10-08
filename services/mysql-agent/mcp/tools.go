package mcp

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"mysql-agent/config"

	_ "github.com/go-sql-driver/mysql"
)

// Tool MCP工具接口
type Tool interface {
	GetDefinition() ToolDefinition
	Execute(params map[string]interface{}) (interface{}, error)
}

// ToolDefinition 工具定义
type ToolDefinition struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

// Function 工具函数定义
type Function struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

// Parameters 工具参数定义
type Parameters struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required"`
}

// Property 参数属性
type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// SlowQueryTool 慢查询分析工具
type SlowQueryTool struct {
	db *sql.DB
}

// NewSlowQueryTool 创建慢查询工具
func NewSlowQueryTool() (*SlowQueryTool, error) {
	db, err := getDBConnection()
	if err != nil {
		return nil, err
	}
	return &SlowQueryTool{db: db}, nil
}

func (t *SlowQueryTool) GetDefinition() ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: Function{
			Name:        "slow_query_analysis",
			Description: "分析MySQL慢查询日志，获取执行时间较长的SQL语句信息",
			Parameters: Parameters{
				Type: "object",
				Properties: map[string]Property{
					"limit": {
						Type:        "integer",
						Description: "返回的慢查询条数，默认为10",
					},
				},
				Required: []string{},
			},
		},
	}
}

func (t *SlowQueryTool) Execute(params map[string]interface{}) (interface{}, error) {
	limit := 10
	if l, ok := params["limit"].(float64); ok {
		limit = int(l)
	}

	// 查询慢查询相关信息
	query := `
		SELECT 
			DIGEST_TEXT as query_text,
			COUNT_STAR as exec_count,
			AVG_TIMER_WAIT/1000000000000 as avg_time_seconds,
			MAX_TIMER_WAIT/1000000000000 as max_time_seconds,
			SUM_LOCK_TIME/1000000000000 as total_lock_time_seconds
		FROM performance_schema.events_statements_summary_by_digest 
		WHERE DIGEST_TEXT IS NOT NULL 
		ORDER BY AVG_TIMER_WAIT DESC 
		LIMIT ?
	`

	rows, err := t.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("query slow queries: %w", err)
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var queryText sql.NullString
		var execCount, avgTime, maxTime, lockTime sql.NullFloat64

		err := rows.Scan(&queryText, &execCount, &avgTime, &maxTime, &lockTime)
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		result := map[string]interface{}{
			"query_text":         queryText.String,
			"execution_count":    execCount.Float64,
			"avg_execution_time": avgTime.Float64,
			"max_execution_time": maxTime.Float64,
			"total_lock_time":    lockTime.Float64,
		}
		results = append(results, result)
	}

	return map[string]interface{}{
		"slow_queries": results,
		"total_count":  len(results),
	}, nil
}

// ShowStatusTool 显示MySQL状态工具
type ShowStatusTool struct {
	db *sql.DB
}

// NewShowStatusTool 创建状态显示工具
func NewShowStatusTool() (*ShowStatusTool, error) {
	db, err := getDBConnection()
	if err != nil {
		return nil, err
	}
	return &ShowStatusTool{db: db}, nil
}

func (t *ShowStatusTool) GetDefinition() ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: Function{
			Name:        "show_status",
			Description: "显示MySQL服务器状态信息，包括连接数、查询数等关键指标",
			Parameters: Parameters{
				Type: "object",
				Properties: map[string]Property{
					"pattern": {
						Type:        "string",
						Description: "状态变量名称模式，支持通配符%，如'Conn%'表示所有以Conn开头的变量",
					},
				},
				Required: []string{},
			},
		},
	}
}

func (t *ShowStatusTool) Execute(params map[string]interface{}) (interface{}, error) {
	pattern := ""
	if p, ok := params["pattern"].(string); ok {
		pattern = p
	}

	var query string
	var args []interface{}

	if pattern != "" {
		// MySQL does not support parameter placeholders for SHOW statements in all versions/drivers.
		// 做简单的安全处理：转义单引号，允许通配符 % 和 _ 保持使用
		safe := strings.ReplaceAll(pattern, "'", "''")
		query = fmt.Sprintf("SHOW STATUS LIKE '%s'", safe)
	} else {
		// 默认显示重要的状态变量
		query = `SHOW STATUS WHERE Variable_name IN (
			'Connections', 'Max_used_connections', 'Threads_connected', 'Threads_running',
			'Queries', 'Questions', 'Com_select', 'Com_insert', 'Com_update', 'Com_delete',
			'Bytes_received', 'Bytes_sent', 'Uptime', 'Slow_queries'
		)`
	}

	rows, err := t.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query status: %w", err)
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var name, value string
		err := rows.Scan(&name, &value)
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		results = append(results, map[string]interface{}{
			"variable_name": name,
			"value":         value,
		})
	}

	return map[string]interface{}{
		"status_variables": results,
		"total_count":      len(results),
	}, nil
}

// ConnectionsTool 连接信息工具
type ConnectionsTool struct {
	db *sql.DB
}

// NewConnectionsTool 创建连接信息工具
func NewConnectionsTool() (*ConnectionsTool, error) {
	db, err := getDBConnection()
	if err != nil {
		return nil, err
	}
	return &ConnectionsTool{db: db}, nil
}

func (t *ConnectionsTool) GetDefinition() ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: Function{
			Name:        "show_connections",
			Description: "显示当前MySQL数据库的连接信息，包括活动连接列表和连接统计",
			Parameters: Parameters{
				Type:       "object",
				Properties: map[string]Property{},
				Required:   []string{},
			},
		},
	}
}

func (t *ConnectionsTool) Execute(params map[string]interface{}) (interface{}, error) {
	// 查询当前连接列表
	processQuery := "SHOW PROCESSLIST"
	rows, err := t.db.Query(processQuery)
	if err != nil {
		return nil, fmt.Errorf("query processlist: %w", err)
	}
	defer rows.Close()

	var processes []map[string]interface{}
	for rows.Next() {
		var id sql.NullInt64
		var user, host, db, command, state sql.NullString
		var time sql.NullInt64
		var info sql.NullString

		err := rows.Scan(&id, &user, &host, &db, &command, &time, &state, &info)
		if err != nil {
			return nil, fmt.Errorf("scan processlist row: %w", err)
		}

		process := map[string]interface{}{
			"id":      id.Int64,
			"user":    user.String,
			"host":    host.String,
			"db":      db.String,
			"command": command.String,
			"time":    time.Int64,
			"state":   state.String,
			"info":    info.String,
		}
		processes = append(processes, process)
	}

	// 查询连接统计信息
	statusQuery := `
		SHOW STATUS WHERE Variable_name IN (
			'Threads_connected', 'Threads_running', 'Max_used_connections', 
			'Connections', 'Connection_errors_max_connections'
		)
	`
	statusRows, err := t.db.Query(statusQuery)
	if err != nil {
		return nil, fmt.Errorf("query connection status: %w", err)
	}
	defer statusRows.Close()

	stats := make(map[string]string)
	for statusRows.Next() {
		var name, value string
		err := statusRows.Scan(&name, &value)
		if err != nil {
			return nil, fmt.Errorf("scan status row: %w", err)
		}
		stats[name] = value
	}

	return map[string]interface{}{
		"active_connections":     processes,
		"connection_statistics":  stats,
		"total_active_processes": len(processes),
	}, nil
}

// ProcessListTool 进程列表工具
type ProcessListTool struct {
	db *sql.DB
}

// NewProcessListTool 创建进程列表工具
func NewProcessListTool() (*ProcessListTool, error) {
	db, err := getDBConnection()
	if err != nil {
		return nil, err
	}
	return &ProcessListTool{db: db}, nil
}

func (t *ProcessListTool) GetDefinition() ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: Function{
			Name:        "show_processlist",
			Description: "显示MySQL当前正在执行的进程列表，可以看到正在运行的查询",
			Parameters: Parameters{
				Type: "object",
				Properties: map[string]Property{
					"full": {
						Type:        "boolean",
						Description: "是否显示完整的查询语句，默认为false",
					},
				},
				Required: []string{},
			},
		},
	}
}

func (t *ProcessListTool) Execute(params map[string]interface{}) (interface{}, error) {
	full := false
	if f, ok := params["full"].(bool); ok {
		full = f
	}

	query := "SHOW PROCESSLIST"
	if full {
		query = "SHOW FULL PROCESSLIST"
	}

	rows, err := t.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query processlist: %w", err)
	}
	defer rows.Close()

	var processes []map[string]interface{}
	for rows.Next() {
		var id sql.NullInt64
		var user, host, db, command, state sql.NullString
		var time sql.NullInt64
		var info sql.NullString

		err := rows.Scan(&id, &user, &host, &db, &command, &time, &state, &info)
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		process := map[string]interface{}{
			"id":      id.Int64,
			"user":    user.String,
			"host":    host.String,
			"db":      db.String,
			"command": command.String,
			"time":    time.Int64,
			"state":   state.String,
			"info":    info.String,
		}
		processes = append(processes, process)
	}

	return map[string]interface{}{
		"processes":   processes,
		"total_count": len(processes),
	}, nil
}

// getDBConnection 获取数据库连接
func getDBConnection() (*sql.DB, error) {
	if config.AppConfig == nil {
		return nil, fmt.Errorf("config not initialized")
	}

	cfg := config.AppConfig.MySQL
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// 测试连接
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// 设置连接池参数
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	return db, nil
}

// ToolRegistry 工具注册表
type ToolRegistry struct {
	tools map[string]Tool
}

// NewToolRegistry 创建工具注册表
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]Tool),
	}
}

// RegisterTool 注册工具
func (r *ToolRegistry) RegisterTool(tool Tool) error {
	def := tool.GetDefinition()
	r.tools[def.Function.Name] = tool
	return nil
}

// GetTool 获取工具
func (r *ToolRegistry) GetTool(name string) (Tool, bool) {
	tool, exists := r.tools[name]
	return tool, exists
}

// GetAllTools 获取所有工具
func (r *ToolRegistry) GetAllTools() []Tool {
	var tools []Tool
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

// GetToolDefinitions 获取所有工具定义
func (r *ToolRegistry) GetToolDefinitions() []ToolDefinition {
	var definitions []ToolDefinition
	for _, tool := range r.tools {
		definitions = append(definitions, tool.GetDefinition())
	}
	return definitions
}

// InitializeTools 初始化所有工具
func InitializeTools() (*ToolRegistry, error) {
	registry := NewToolRegistry()

	// 注册慢查询工具
	slowQueryTool, err := NewSlowQueryTool()
	if err != nil {
		return nil, fmt.Errorf("create slow query tool: %w", err)
	}
	registry.RegisterTool(slowQueryTool)

	// 注册状态显示工具
	showStatusTool, err := NewShowStatusTool()
	if err != nil {
		return nil, fmt.Errorf("create show status tool: %w", err)
	}
	registry.RegisterTool(showStatusTool)

	// 注册连接信息工具
	connectionsTool, err := NewConnectionsTool()
	if err != nil {
		return nil, fmt.Errorf("create connections tool: %w", err)
	}
	registry.RegisterTool(connectionsTool)

	// 注册进程列表工具
	processListTool, err := NewProcessListTool()
	if err != nil {
		return nil, fmt.Errorf("create processlist tool: %w", err)
	}
	registry.RegisterTool(processListTool)

	return registry, nil
}
