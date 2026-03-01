package store

import (
	"database/sql"
	"fmt"
)

// CreateAccount 创建账号
func (s *Store) CreateAccount(username, password string) (*Account, error) {
	hashed := HashPassword(password)
	result, err := s.db.Exec(
		"INSERT INTO accounts (username, password) VALUES (?, ?)",
		username, hashed,
	)
	if err != nil {
		return nil, fmt.Errorf("用户名已存在或创建失败: %w", err)
	}
	id, _ := result.LastInsertId()
	return &Account{ID: id, Username: username}, nil
}

// Authenticate 验证登录
func (s *Store) Authenticate(username, password string) (*Account, error) {
	hashed := HashPassword(password)
	var acc Account
	err := s.db.QueryRow(
		"SELECT id, username FROM accounts WHERE username=? AND password=?",
		username, hashed,
	).Scan(&acc.ID, &acc.Username)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("用户名或密码错误")
	}
	if err != nil {
		return nil, err
	}
	return &acc, nil
}
