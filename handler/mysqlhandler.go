package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"mysql-backend/models"
	"mysql-backend/request"
	"mysql-backend/service"
)

// CreateMySQLUser 处理创建MySQL用户的请求
func CreateMySQLUser(c *gin.Context) {
	req := &request.CreateUserRequest{}

	// 绑定请求参数
	if err := c.ShouldBindJSON(req); err != nil {
		response := models.StandardResponse{
			Data:         models.CreateUserResponse{Success: false},
			Error:        "INVALID_REQUEST",
			ErrorMessage: err.Error(),
		}
		c.JSON(http.StatusBadRequest, response)
		return
	}

	// 验证请求参数
	if err := req.Validate(); err != nil {
		response := models.StandardResponse{
			Data:         models.CreateUserResponse{Success: false},
			Error:        "VALIDATION_ERROR",
			ErrorMessage: err.Error(),
		}
		c.JSON(http.StatusBadRequest, response)
		return
	}

	req.Ctx = c.Request.Context()

	// 调用service方法处理业务逻辑（只传入req）
	response := service.CreateUser(*req)

	// 根据响应中的error字段判断HTTP状态码
	statusCode := http.StatusOK
	if response.Error != "NO_ERROR" {
		statusCode = http.StatusInternalServerError
	}

	// 返回统一响应格式
	c.JSON(statusCode, response)
}

func CheckMySQLUser(c *gin.Context) {
	req := &request.CheckUserRequst{}

	if err := c.ShouldBindJSON(req); err != nil {
		response := models.StandardResponse{
			Data:         nil,
			Error:        "INVALID_REQUEST",
			ErrorMessage: err.Error(),
		}

		c.JSON(http.StatusBadRequest, response)
		return
	}

	req.Ctx = c.Request.Context()

	response := service.CheckUser(*req)
	statusCode := http.StatusOK
	if response.Error != "NO_ERROR" {
		statusCode = http.StatusInternalServerError
	}

	// 返回统一响应格式
	c.JSON(statusCode, response)
}
