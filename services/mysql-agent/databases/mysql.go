package databases

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"

	mysql "github.com/go-sql-driver/mysql"

	"mysql-agent/config"
)

var (
	dbInstance *sql.DB
	dbMu       sync.RWMutex
)

func InitDB() error {
	dbMu.Lock()
	defer dbMu.Unlock()
	if dbInstance != nil {
		return nil
	}

	dsn := config.AppConfig.GetDSN()
	conn, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("打开mysql失败: %w", err)
	}

	if config.AppConfig.Database.MaxIdleConns > 0 {
		conn.SetMaxIdleConns(config.AppConfig.Database.MaxIdleConns)
	}
	if config.AppConfig.Database.MaxOpenConns > 0 {
		conn.SetMaxOpenConns(config.AppConfig.Database.MaxOpenConns)
	}
	if config.AppConfig.Database.ConnMaxLifetime > 0 {
		conn.SetConnMaxLifetime(config.AppConfig.Database.ConnMaxLifetime)
	}

	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return fmt.Errorf("尝试ping数据库失败: %w", err)
	}

	dbInstance = conn
	return nil
}

func GetDB() (*sql.DB, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()
	if dbInstance == nil {
		return nil, fmt.Errorf("数据库未初始化")
	}
	return dbInstance, nil
}

func CloseDB() error {
	dbMu.Lock()
	defer dbMu.Unlock()
	if dbInstance == nil {
		return nil
	}
	err := dbInstance.Close()
	dbInstance = nil
	return err
}

func QueryProcessList(ctx context.Context) ([]map[string]any, error) {
	db, err := GetDB()
	if err != nil {
		return nil, err
	}

	return queryWithFallback(ctx, db, "SHOW FULL PROCESSLIST", "SHOW PROCESSLIST", shouldFallback)
}

func QueryInnoDBStatus(ctx context.Context) ([]map[string]any, error) {
	db, err := GetDB()
	if err != nil {
		return nil, err
	}

	return queryWithFallback(ctx, db, "SHOW ENGINE INNODB STATUS", "SHOW INNODB STATUS", shouldFallbackInnoDBSyntax)
}

func QueryGlobalStatus(ctx context.Context) ([]map[string]any, error) {
	db, err := GetDB()
	if err != nil {
		return nil, err
	}

	return querySimple(ctx, db, "SHOW GLOBAL STATUS")
}

func QueryInnoDBTrx(ctx context.Context, limit int) ([]map[string]any, error) {
	db, err := GetDB()
	if err != nil {
		return nil, err
	}

	query := "SELECT * FROM information_schema.innodb_trx ORDER BY trx_started"
	var args []any
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	return querySimple(ctx, db, query, args...)
}

func QueryInnoDBMutex(ctx context.Context) ([]map[string]any, error) {
	db, err := GetDB()
	if err != nil {
		return nil, err
	}

	return querySimple(ctx, db, "SHOW ENGINE INNODB MUTEX")
}

func QuerySlowQueries(ctx context.Context, limit int) ([]map[string]any, error) {
	db, err := GetDB()
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 20
	}

	query := `SELECT DIGEST_TEXT, SCHEMA_NAME, COUNT_STAR, SUM_TIMER_WAIT, AVG_TIMER_WAIT, SUM_ERRORS, SUM_WARNINGS, SUM_ROWS_AFFECTED, SUM_ROWS_SENT, SUM_ROWS_EXAMINED, FIRST_SEEN, LAST_SEEN` +
		" FROM performance_schema.events_statements_summary_by_digest\n" +
		"WHERE DIGEST_TEXT IS NOT NULL\n" +
		"ORDER BY SUM_TIMER_WAIT DESC\n" +
		"LIMIT ?"

	return querySimple(ctx, db, query, limit)
}

func QuerySchemaStats(ctx context.Context, schema string, limit int) ([]map[string]any, error) {
	db, err := GetDB()
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(schema) == "" {
		schema = config.AppConfig.Database.DBName
	}

	query := `SELECT TABLE_SCHEMA, TABLE_NAME, ENGINE, TABLE_ROWS, DATA_LENGTH, INDEX_LENGTH, DATA_LENGTH + INDEX_LENGTH AS TOTAL_LENGTH, AUTO_INCREMENT, UPDATE_TIME` +
		" FROM information_schema.tables\n" +
		"WHERE TABLE_SCHEMA = ?\n" +
		"ORDER BY TOTAL_LENGTH DESC"

	args := []any{schema}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	return querySimple(ctx, db, query, args...)
}

func QueryGlobalVariables(ctx context.Context) (map[string]string, error) {
	db, err := GetDB()
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, "SHOW VARIABLES")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			return nil, err
		}
		result[strings.ToLower(name)] = value
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func shouldFallback(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		switch mysqlErr.Number {
		case 2020, 2027, 2028:
			return true
		}
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "too much") || strings.Contains(msg, "too many") || strings.Contains(msg, "max_allowed_packet") || strings.Contains(msg, "data too long")
}

func shouldFallbackInnoDBSyntax(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		if mysqlErr.Number == 1064 {
			return true
		}
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "syntax")
}

func queryWithFallback(ctx context.Context, db *sql.DB, primary, fallback string, fallbackCond func(error) bool) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, primary)
	if err != nil {
		if fallback == "" || fallbackCond == nil || !fallbackCond(err) {
			return nil, err
		}
		rows, err = db.QueryContext(ctx, fallback)
		if err != nil {
			return nil, err
		}
	}
	defer rows.Close()

	return scanRows(rows)
}

func querySimple(ctx context.Context, db *sql.DB, query string, args ...any) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRows(rows)
}

func scanRows(rows *sql.Rows) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	values := make([]any, len(cols))
	args := make([]any, len(cols))
	for i := range values {
		args[i] = &values[i]
	}

	var result []map[string]any
	for rows.Next() {
		if err := rows.Scan(args...); err != nil {
			return nil, err
		}

		row := make(map[string]any, len(cols))
		for i, col := range cols {
			switch v := values[i].(type) {
			case []byte:
				row[col] = string(v)
			default:
				row[col] = v
			}
		}
		result = append(result, row)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}
