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
	Answer  string                 `json:"answer"`
	Sources []AgentSource          `json:"sources"`
	Raw     map[string]interface{} `json:"raw"`
}

type AgentSource struct {
	Tool        string                 `json:"tool"`
	Description string                 `json:"description,omitempty"`
	Status      string                 `json:"status"`
	Params      map[string]interface{} `json:"params,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

type UserInfo struct {
	Exist     bool     `json:"exist"`
	DB        string   `json:"db"`
	Privilege []string `json:"privilege"`
	Plugins   []string `json:"plugins"`
}
