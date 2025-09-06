package service

import (
	"context"
	"fmt"
	"strings"

	"mysql-backend/databases"
	"mysql-backend/models"
	"mysql-backend/request"
) // CreateUserWithPrivileges 创建或更新用户并授予权限

func CreateUserWithPrivileges(ctx context.Context, req request.CreateUserRequest) error {
	db, err := databases.GetAdminDB()
	if err != nil {
		return err
	}

	userIdent := fmt.Sprintf("'%s'@'%s'", req.Username, req.Host)

	// CREATE USER IF NOT EXISTS + IDENTIFIED BY '...'
	createStmt := fmt.Sprintf("CREATE USER IF NOT EXISTS %s IDENTIFIED BY '%s'", userIdent, escapeSQLString(req.Password))
	if req.TLSRequire {
		createStmt += " REQUIRE SSL"
	}
	if _, err := db.ExecContext(ctx, createStmt); err != nil {
		return fmt.Errorf("create user failed: %w", err)
	}

	// ALTER USER 确保更新密码/SSL
	alterStmt := fmt.Sprintf("ALTER USER %s IDENTIFIED BY '%s'", userIdent, escapeSQLString(req.Password))
	if req.TLSRequire {
		alterStmt += " REQUIRE SSL"
	}
	if _, err := db.ExecContext(ctx, alterStmt); err != nil {
		return fmt.Errorf("alter user failed: %w", err)
	}

	// 权限列表
	privs := make([]string, 0, len(req.Privileges))
	for _, p := range req.Privileges {
		privs = append(privs, string(p))
	}
	privList := strings.Join(privs, ", ")

	// 对每个数据库授权
	for _, dbName := range req.Databases {
		scope := "*.*"
		if dbName != "*" {
			safe := strings.TrimSpace(dbName)
			if safe == "" {
				continue
			}
			scope = fmt.Sprintf("`%s`.*", strings.ReplaceAll(safe, "`", ""))
		}

		grant := fmt.Sprintf("GRANT %s ON %s TO %s", privList, scope, userIdent)
		if req.WithGrant {
			grant += " WITH GRANT OPTION"
		}
		if _, err := db.ExecContext(ctx, grant); err != nil {
			return fmt.Errorf("grant on %s failed: %w", scope, err)
		}
	}

	// 刷新权限
	if _, err := db.ExecContext(ctx, "FLUSH PRIVILEGES"); err != nil {
		return fmt.Errorf("flush privileges failed: %w", err)
	}

	return nil
}

// CreateUser 处理创建用户的业务逻辑，返回统一响应
func CreateUser(req request.CreateUserRequest) models.StandardResponse {
	if err := CreateUserWithPrivileges(req.Ctx, req); err != nil {
		return models.StandardResponse{
			Data:         models.CreateUserResponse{Success: false},
			Error:        "OPERATION_FAILED",
			ErrorMessage: err.Error(),
		}
	}

	return models.StandardResponse{
		Data:         models.CreateUserResponse{Success: true},
		Error:        "NO_ERROR",
		ErrorMessage: "Operation completed successfully",
	}
}

// escapeSQLString 简单转义用于单引号包裹的 MySQL 字符串字面量
func escapeSQLString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	return s
}
