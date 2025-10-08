package request

import (
	"context"
	"errors"
	"fmt"
	"regexp"
)

type Privilege string

// 常见权限集合
var allowedPrivileges = map[Privilege]struct{}{
	"ALL":                     {},
	"SELECT":                  {},
	"INSERT":                  {},
	"UPDATE":                  {},
	"DELETE":                  {},
	"CREATE":                  {},
	"DROP":                    {},
	"RELOAD":                  {},
	"SHUTDOWN":                {},
	"PROCESS":                 {},
	"FILE":                    {},
	"GRANT OPTION":            {},
	"REFERENCES":              {},
	"INDEX":                   {},
	"ALTER":                   {},
	"SHOW DATABASES":          {},
	"SUPER":                   {},
	"CREATE TEMPORARY TABLES": {},
	"LOCK TABLES":             {},
	"EXECUTE":                 {},
	"REPLICATION SLAVE":       {},
	"REPLICATION CLIENT":      {},
	"CREATE VIEW":             {},
	"SHOW VIEW":               {},
	"CREATE ROUTINE":          {},
	"ALTER ROUTINE":           {},
	"CREATE USER":             {},
	"EVENT":                   {},
	"TRIGGER":                 {},
}

// CreateUserRequest 定义创建用户的请求体
type CreateUserRequest struct {
	Username   string      `json:"username"`    // 新用户用户名
	Host       string      `json:"host"`        // 允许连接的host，默认"%"
	Password   string      `json:"password"`    // 用户密码
	Databases  []string    `json:"databases"`   // 授权的数据库列表，例如["db1","db2"]，支持通配符"*"
	Privileges []Privilege `json:"privileges"`  // 权限列表，例如["SELECT","INSERT"]或["ALL"]
	WithGrant  bool        `json:"with_grant"`  // 是否包含 GRANT OPTION
	TLSRequire bool        `json:"tls_require"` // 是否需要 REQUIRE SSL

	Ctx context.Context `json:"-"` // 请求上下文
}

type CheckUserRequst struct {
	Username []string `json:"usernames"`

	Ctx context.Context `json:"-"`
}

func (r *CreateUserRequest) Validate() error {
	if r.Username == "" {
		return errors.New("username is required")
	}
	if r.Password == "" {
		return errors.New("password is required")
	}
	if r.Host == "" {
		r.Host = "%"
	}
	if len(r.Databases) == 0 {
		r.Databases = []string{"*"}
	}
	// 用户名与host格式校验（基础）
	if !regexp.MustCompile(`^[A-Za-z0-9_\-\.]+$`).MatchString(r.Username) {
		return fmt.Errorf("invalid username: %s", r.Username)
	}
	// 权限校验
	if len(r.Privileges) == 0 {
		r.Privileges = []Privilege{"ALL"}
	}
	for _, p := range r.Privileges {
		if _, ok := allowedPrivileges[p]; !ok {
			return fmt.Errorf("invalid privilege: %s", p)
		}
	}
	return nil
}
