package main

import (
	"log"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"

	"mysql-agent/agent"
	"mysql-agent/config"
)

func main() {
	// 初始化配置
	config.InitConfig()

	rpcServer := rpc.NewServer()
	if err := rpcServer.RegisterName("Agent", &agent.Service{}); err != nil {
		log.Fatalf("注册 RPC 服务失败: %v", err)
	}

	addr := config.AppConfig.GetServerAddr()
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("监听 %s 失败: %v", addr, err)
	}
	log.Printf("RPC 服务器已启动，监听地址: %s", addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				log.Printf("临时连接错误: %v", err)
				continue
			}
			log.Fatalf("接受连接失败: %v", err)
		}

		go rpcServer.ServeCodec(jsonrpc.NewServerCodec(conn))
	}
}
