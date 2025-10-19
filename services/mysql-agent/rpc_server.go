package main

import (
	"context"
	"errors"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"

	"mysql-agent/agent"
	"mysql-agent/config"
)

func runRPCServer(ctx context.Context) error {
	addr := ":" + config.AppConfig.Server.Port
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	srv := rpc.NewServer()
	if err := agent.RegisterRPC(srv); err != nil {
		return err
	}

	errCh := make(chan error, 1)

	go func() {
		errCh <- acceptLoop(ctx, srv, listener)
	}()

	select {
	case <-ctx.Done():
		_ = listener.Close()
		return nil
	case err := <-errCh:
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		if err != nil && ctx.Err() != nil {
			return nil
		}
		return err
	}
}

func acceptLoop(ctx context.Context, srv *rpc.Server, listener net.Listener) error {
	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return net.ErrClosed
			default:
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Temporary() {
				continue
			}
			return err
		}

		go srv.ServeCodec(jsonrpc.NewServerCodec(conn))
	}
}
