package models

// StandardResponse 统一响应结构
type StandardResponse struct {
	Data         interface{} `json:"data"`
	Error        string      `json:"error"`
	ErrorMessage string      `json:"error_message"`
}

// CreateUserResponse 创建用户的响应数据
type CreateUserResponse struct {
	Success bool `json:"success"`
}

type CheckUserResponse struct {
	UserInfos []UserInfo `json:"user_infos"`
}

type AgentQueryResponse struct {
	Analysis AgentAnalysis          `json:"analysis"`
	ToolRuns []AgentToolRun         `json:"tool_runs"`
	Raw      map[string]interface{} `json:"raw,omitempty"`
}

type AgentAnalysis struct {
	Summary string `json:"summary,omitempty"`
	Error   string `json:"error,omitempty"`
}

type AgentToolRun struct {
	Name       string      `json:"name"`
	Reason     string      `json:"reason,omitempty"`
	Input      interface{} `json:"input,omitempty"`
	Output     interface{} `json:"output,omitempty"`
	Error      string      `json:"error,omitempty"`
	DurationMs int64       `json:"duration_ms"`
}

type UserInfo struct {
	Exist     bool     `json:"exist"`
	DB        string   `json:"db"`
	Privilege []string `json:"privilege"`
	Plugins   []string `json:"plugins"`
}
