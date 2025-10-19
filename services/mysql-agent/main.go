package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"mysql-agent/agent"
	"mysql-agent/config"
	"mysql-agent/databases"
)

func main() {
	config.InitConfig()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := databases.InitDB(); err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	defer func() {
		if err := databases.CloseDB(); err != nil {
			log.Printf("关闭数据库失败: %v", err)
		}
	}()

	if _, err := agent.ChatModel(ctx); err != nil {
		log.Fatalf("初始化deepseek模型失败: %v", err)
	}
	if names, err := agent.ToolNames(ctx); err != nil {
		log.Printf("注册工具失败: %v", err)
	} else {
		log.Printf("已注册工具: %v", names)
	}

	log.Printf("RPC 服务监听: %s", config.AppConfig.GetServerAddr())
	log.Printf("数据库DSN: %s", config.AppConfig.GetDSN())

	if err := runRPCServer(ctx); err != nil {
		log.Fatalf("服务运行失败: %v", err)
	}
}
