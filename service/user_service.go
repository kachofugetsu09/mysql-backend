package service

import (
	"context"
	"fmt"
	"mysql-backend/helper"
	"strings"

	"mysql-backend/databases"
	"mysql-backend/models"
	"mysql-backend/request"
)

// CreateUserWithPrivileges 创建或更新用户并授予权限
func CreateUserWithPrivileges(ctx context.Context, req request.CreateUserRequest) error {
	db, err := databases.GetAdminDB()
	if err != nil {
		return err
	}

	userIdent := fmt.Sprintf("'%s'@'%s'", req.Username, req.Host)

	// CREATE USER IF NOT EXISTS + IDENTIFIED BY '...'
	createStmt := fmt.Sprintf("CREATE USER IF NOT EXISTS %s IDENTIFIED BY '%s'", userIdent, helper.EscapeSQLString(req.Password))
	if _, err := db.ExecContext(ctx, createStmt); err != nil {
		return fmt.Errorf("create user failed: %w", err)
	}

	// ALTER USER 确保更新密码/SSL
	alterStmt := fmt.Sprintf("ALTER USER %s IDENTIFIED BY '%s'", userIdent, helper.EscapeSQLString(req.Password))
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

func CheckUser(req request.CheckUserRequst) models.StandardResponse {
	resp, err := CheckUserWithId(req.Ctx, req)
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

func CheckUserWithId(ctx context.Context, req request.CheckUserRequst) (models.CheckUserResponse, error) {
	// 如果用户名列表为空，直接返回空结果
	if len(req.Username) == 0 {
		return models.CheckUserResponse{UserInfos: []models.UserInfo{}}, nil
	}

	db, err := databases.GetAdminDB()
	if err != nil {
		return models.CheckUserResponse{}, err
	}
	userinfos := make([]models.UserInfo, 0)
	for _, username := range req.Username {
		var userinfo models.UserInfo

		// 检查用户是否存在
		existQuery := "SELECT EXISTS(SELECT 1 FROM mysql.user WHERE user = ?)"
		var exist bool
		if err := db.QueryRowContext(ctx, existQuery, username).Scan(&exist); err != nil {
			return models.CheckUserResponse{}, err
		}
		if !exist {
			userinfo.Exist = false
			userinfos = append(userinfos, userinfo)
			continue
		}
		userinfo.Exist = true

		// 查询 host 与 auth plugin（可能多条）
		hostRows, err := db.QueryContext(ctx, "SELECT host, plugin FROM mysql.user WHERE user = ?", username)
		if err != nil {
			return models.CheckUserResponse{}, err
		}
		hosts := make([]string, 0)
		plugins := make([]string, 0)
		for hostRows.Next() {
			var host, plugin string
			if err := hostRows.Scan(&host, &plugin); err != nil {
				hostRows.Close()
				return models.CheckUserResponse{}, err
			}
			hosts = append(hosts, host)
			if strings.TrimSpace(plugin) != "" {
				plugins = append(plugins, plugin)
			}
		}
		if err := hostRows.Err(); err != nil {
			hostRows.Close()
			return models.CheckUserResponse{}, err
		}
		hostRows.Close()

		// SHOW GRANTS for each host, 聚合权限
		allGrants := make([]string, 0)
		for _, host := range hosts {
			uEsc := helper.EscapeSQLString(username)
			hEsc := helper.EscapeSQLString(host)
			query := fmt.Sprintf("SHOW GRANTS FOR '%s'@'%s'", uEsc, hEsc)

			rows, err := db.QueryContext(ctx, query)
			if err != nil {
				return models.CheckUserResponse{}, err
			}

			for rows.Next() {
				var grant string
				if err := rows.Scan(&grant); err != nil {
					rows.Close()
					return models.CheckUserResponse{}, err
				}
				allGrants = append(allGrants, grant)
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return models.CheckUserResponse{}, err
			}
			rows.Close()
		}

		allGrants = helper.UniqueStrings(allGrants)

		// 解析权限列表
		userinfo.Privilege = helper.ParsePrivilegesFromGrants(allGrants)

		// 解析数据库列表
		dbs := helper.ParseDatabasesFromGrants(allGrants)
		if len(dbs) == 0 {
			userinfo.DB = ""
		} else if len(dbs) == 1 && dbs[0] == "*" {
			userinfo.DB = "*"
		} else {
			userinfo.DB = strings.Join(dbs, ",")
		}

		// 设置插件列表
		userinfo.Plugins = helper.UniqueStrings(plugins)

		userinfos = append(userinfos, userinfo)
	}

	return models.CheckUserResponse{UserInfos: userinfos}, nil
}
