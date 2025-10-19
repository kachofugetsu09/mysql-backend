package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"time"

	"mysql-backend/config"
	"mysql-backend/models"
	"mysql-backend/request"
)

type agentToolCall struct {
	Name   string          `json:"name"`
	Args   json.RawMessage `json:"args,omitempty"`
	Reason string          `json:"reason,omitempty"`
}

type agentRPCRequest struct {
	Query          string            `json:"query"`
	Tools          []agentToolCall   `json:"tools,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
	Context        map[string]string `json:"context,omitempty"`
}

func QueryAgent(req request.AgentQueryRequest) models.StandardResponse {
	resp, err := queryAgent(req.Ctx, req)

	if err != nil {
		return models.StandardResponse{
			Data:         nil,
			Error:        "OPERATION_FAILED",
			ErrorMessage: err.Error(),
		}
	}
	return models.StandardResponse{
		Data:         resp,
		Error:        "NO_ERROR",
		ErrorMessage: "Operation completed successfully",
	}
}

func queryAgent(ctx context.Context, req request.AgentQueryRequest) (models.AgentQueryResponse, error) {
	if config.AppConfig == nil {
		return models.AgentQueryResponse{}, fmt.Errorf("config is not initialised")
	}

	agentCfg := config.AppConfig.Agent
	rpcAddr := config.AppConfig.GetAgentRPCAddr()

	dialer := &net.Dialer{}
	if agentCfg.Timeout > 0 {
		dialer.Timeout = agentCfg.Timeout
	}

	conn, err := dialer.DialContext(ctx, "tcp", rpcAddr)
	if err != nil {
		return models.AgentQueryResponse{}, fmt.Errorf("dial mysql-agent rpc: %w", err)
	}
	defer conn.Close()

	deadline, hasDeadline := ctx.Deadline()
	if !hasDeadline && agentCfg.Timeout > 0 {
		deadline = time.Now().Add(agentCfg.Timeout)
		hasDeadline = true
	}

	if hasDeadline {
		if err := conn.SetDeadline(deadline); err != nil {
			return models.AgentQueryResponse{}, fmt.Errorf("set deadline: %w", err)
		}
	}

	client := rpc.NewClientWithCodec(jsonrpc.NewClientCodec(conn))
	defer client.Close()

	toolCalls := make([]agentToolCall, 0, len(req.Tools))
	for _, t := range req.Tools {
		toolCalls = append(toolCalls, agentToolCall{Name: t.Name, Args: t.Args, Reason: t.Reason})
	}

	timeoutSeconds := req.TimeoutSeconds
	if timeoutSeconds <= 0 && agentCfg.Timeout > 0 {
		timeoutSeconds = int(agentCfg.Timeout / time.Second)
	}

	rpcReq := agentRPCRequest{
		Query:          req.Query,
		Tools:          toolCalls,
		TimeoutSeconds: timeoutSeconds,
		Context:        req.Context,
	}

	var rpcResp models.AgentQueryResponse
	done := make(chan error, 1)
	go func() {
		done <- client.Call("Agent.Query", rpcReq, &rpcResp)
	}()

	select {
	case <-ctx.Done():
		_ = conn.Close()
		return models.AgentQueryResponse{}, fmt.Errorf("rpc call canceled: %w", ctx.Err())
	case err := <-done:
		if err != nil {
			return models.AgentQueryResponse{}, fmt.Errorf("call Agent.Query: %w", err)
		}
	}

	return rpcResp, nil
}
