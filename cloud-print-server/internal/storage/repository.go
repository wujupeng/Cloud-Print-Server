package storage

import (
	"context"
	"database/sql"
	"time"
)

type Repository struct {
	db *DB
}

func NewRepository(db *DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) DB() *DB { return r.db }

func (r *Repository) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return r.db.ExecContext(ctx, query, args...)
}

func (r *Repository) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return r.db.QueryContext(ctx, query, args...)
}

func (r *Repository) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return r.db.QueryRowContext(ctx, query, args...)
}

func (r *Repository) WithTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, d)
}