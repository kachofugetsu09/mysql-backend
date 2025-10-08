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
	Answer string `json:"answer"`
}

type UserInfo struct {
	Exist     bool     `json:"exist"`
	DB        string   `json:"db"`
	Privilege []string `json:"privilege"`
	Plugins   []string `json:"plugins"`
}
