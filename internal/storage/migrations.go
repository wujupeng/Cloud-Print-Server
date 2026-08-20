package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func RunMigrations(dbPath string, migrationsDir string) error {
	if _, err := os.Stat(migrationsDir); err != nil {
		return fmt.Errorf("migrations dir 不存在: %w", err)
	}
	db, err := Open(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := ensureSchemaVersions(db); err != nil {
		return fmt.Errorf("ensure schema_versions: %w", err)
	}

	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.up.sql"))
	if err != nil {
		return fmt.Errorf("glob migrations: %w", err)
	}
	sort.Strings(files)

	for _, f := range files {
		base := filepath.Base(f)
		version := strings.TrimSuffix(base, ".up.sql")
		applied, err := isApplied(db, version)
		if err != nil {
			return fmt.Errorf("check applied %s: %w", version, err)
		}
		if applied {
			continue
		}
		sqlBytes, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin tx: %w", err)
		}
		if _, err := tx.Exec(string(sqlBytes)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("exec migration %s: %w", version, err)
		}
		if err := markApplied(tx, version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("mark applied %s: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit %s: %w", version, err)
		}
	}
	return nil
}

func ensureSchemaVersions(db *DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_versions (
		version TEXT PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	return err
}

func isApplied(db *DB, version string) (bool, error) {
	var v string
	err := db.QueryRow(`SELECT version FROM schema_versions WHERE version = ?`, version).Scan(&v)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func markApplied(tx *sql.Tx, version string) error {
	_, err := tx.Exec(`INSERT INTO schema_versions (version) VALUES (?)`, version)
	return err
}