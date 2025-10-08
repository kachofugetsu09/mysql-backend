package router

import (
	"github.com/gin-gonic/gin"
	"mysql-backend/handler"
)

// RegisterRoutes 注册项目的所有HTTP路由
func RegisterRoutes(r *gin.Engine) {
	// 注册路由
	r.POST("/api/mysql/user/create", handler.CreateMySQLUser)
	r.GET("/api/mysql/user/check", handler.CheckMySQLUser)
	r.POST("/api/agent/query", handler.QueryAgent)
}
