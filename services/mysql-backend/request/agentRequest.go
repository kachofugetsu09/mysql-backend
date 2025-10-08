package request

import "context"

type AgentQueryRequest struct {
	Query string `json:"query"` // 查询文本

	Ctx context.Context `json:"-"`
}
