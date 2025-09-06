package main

import (
	"fmt"
	"log"
	"mysql-backend/config"
	"mysql-backend/databases"
	"mysql-backend/router"

	"github.com/gin-gonic/gin"
)

func main() {
	// 初始化配置
	config.InitConfig()

	// 设置Gin模式
	gin.SetMode(config.AppConfig.Server.Mode)
	r := gin.New()

	// 注册业务路由
	router.RegisterRoutes(r)

	// 初始化数据库连接池
	if err := databases.InitAdminDB(); err != nil {
		log.Fatalf("failed to init db: %v", err)
	}
	defer func() {
		if err := databases.CloseAdminDB(); err != nil {
			log.Printf("close db error: %v", err)
		}
	}()

	// 启动服务器
	addr := config.AppConfig.GetServerAddr()
	fmt.Printf("服务器启动在地址: %s\n", addr)
	fmt.Printf("数据库DSN: %s\n", config.AppConfig.GetDSN())
	fmt.Printf("Redis地址: %s\n", config.AppConfig.GetRedisAddr())

	log.Fatal(r.Run(":" + config.AppConfig.Server.Port))
}
