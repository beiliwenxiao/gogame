package store

import (
	"crypto/sha256"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Store 数据存储抽象层
type Store struct {
	db     *sql.DB
	driver string
}

// NewStore 创建数据存储实例。driver: "sqlite"/"mysql", dsn: 数据库路径或连接串
func NewStore(driver, dsn string) (*Store, error) {
	var dbDriver string
	switch driver {
	case "sqlite":
		dbDriver = "sqlite"
	case "mysql":
		dbDriver = "mysql"
	default:
		return nil, fmt.Errorf("不支持的数据库驱动: %s", driver)
	}
	db, err := sql.Open(dbDriver, dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}
	return &Store{db: db, driver: driver}, nil
}

// Close 关闭数据库连接
func (s *Store) Close() error {
	return s.db.Close()
}

// HashPassword 简单密码哈希
func HashPassword(password string) string {
	h := sha256.Sum256([]byte(password))
	return fmt.Sprintf("%x", h)
}
