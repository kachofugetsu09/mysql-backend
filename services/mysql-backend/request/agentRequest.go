package request

import "context"

type AgentQueryRequest struct {
	query string `json:"query"`

	Ctx context.Context `json:"-"`
}
