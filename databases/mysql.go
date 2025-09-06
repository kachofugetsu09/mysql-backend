package databases

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"mysql-backend/config"
)

var (
	adminDB *sql.DB
	dbMu    sync.RWMutex
)

// 初始化
func InitAdminDB() error {
	dbMu.Lock()
	defer dbMu.Unlock()
	if adminDB != nil {
		return nil
	}

	dsn := config.AppConfig.GetAdminDSN()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("打开mysql失败: %w", err)
	}

	// 设置连接池参数
	if config.AppConfig.Database.MaxIdleConns > 0 {
		db.SetMaxIdleConns(config.AppConfig.Database.MaxIdleConns)
	}
	if config.AppConfig.Database.MaxOpenConns > 0 {
		db.SetMaxOpenConns(config.AppConfig.Database.MaxOpenConns)
	}
	if config.AppConfig.Database.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(time.Duration(config.AppConfig.Database.ConnMaxLifetime))
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return fmt.Errorf("尝试ping数据库失败: %w", err)
	}

	adminDB = db
	return nil
}

func GetAdminDB() (*sql.DB, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()
	if adminDB == nil {
		return nil, fmt.Errorf("没有生成adminDB")
	}
	return adminDB, nil
}

func CloseAdminDB() error {
	dbMu.Lock()
	defer dbMu.Unlock()
	if adminDB == nil {
		return nil
	}
	err := adminDB.Close()
	adminDB = nil
	return err
}
