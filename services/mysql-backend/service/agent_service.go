package service

import (
	"context"
	"fmt"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"time"

	"mysql-backend/config"
	"mysql-backend/models"
	"mysql-backend/request"
)

type agentRPCRequest struct {
	Query      string            `json:"query"`
	InstanceID string            `json:"instance"`
	Params     map[string]string `json:"params,omitempty"`
}

type agentRPCResponse struct {
	Answer string `json:"answer"`
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

	rpcReq := agentRPCRequest{
		Query: req.Query,
	}

	var rpcResp agentRPCResponse
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

	return models.AgentQueryResponse{Answer: rpcResp.Answer}, nil
}
