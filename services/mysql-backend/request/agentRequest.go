package request

import (
	"context"
	"encoding/json"
)

type AgentToolCall struct {
	Name   string          `json:"name"`
	Args   json.RawMessage `json:"args,omitempty"`
	Reason string          `json:"reason,omitempty"`
}

type AgentQueryRequest struct {
	Query          string            `json:"query"`
	Tools          []AgentToolCall   `json:"tools,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
	Context        map[string]string `json:"context,omitempty"`

	Ctx context.Context `json:"-"`
}
