package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
	path string
}

func Open(dbPath string) (*DB, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("db path 为空")
	}
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir db dir: %w", err)
	}

	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)", dbPath)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
	}
	for _, p := range pragmas {
		if _, err := sqlDB.Exec(p); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("exec %s: %w", p, err)
		}
	}

	if err := os.Chmod(dbPath, 0o600); err != nil {
		// 非致命，文件可能尚未创建
	}

	return &DB{DB: sqlDB, path: dbPath}, nil
}

func (d *DB) Close() error {
	return d.DB.Close()
}

func (d *DB) Path() string {
	return d.path
}