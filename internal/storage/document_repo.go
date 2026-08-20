package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/cloud-print/server/internal/domain"
)

type DocumentRepo struct {
	*Repository
}

func NewDocumentRepo(db *DB) *DocumentRepo {
	return &DocumentRepo{Repository: NewRepository(db)}
}

func (r *DocumentRepo) Create(ctx context.Context, d *domain.Document) error {
	now := time.Now().UTC()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	_, err := r.ExecContext(ctx, `INSERT INTO documents
		(doc_id, user_id, filename, content_type, size, checksum, storage_path, cleanup_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.DocID, d.UserID, d.Filename, d.ContentType, d.Size, d.Checksum, d.StoragePath, d.CleanupAt, d.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert document: %w", err)
	}
	return nil
}

func (r *DocumentRepo) GetByID(ctx context.Context, docID string) (*domain.Document, error) {
	d := &domain.Document{}
	var contentType sql.NullString
	var cleanup sql.NullTime
	err := r.QueryRowContext(ctx, `SELECT doc_id, user_id, filename, content_type, size, checksum, storage_path, cleanup_at, created_at
		FROM documents WHERE doc_id=?`, docID).Scan(
		&d.DocID, &d.UserID, &d.Filename, &contentType, &d.Size, &d.Checksum, &d.StoragePath, &cleanup, &d.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}
	d.ContentType = contentType.String
	if cleanup.Valid {
		d.CleanupAt = cleanup.Time
	}
	return d, nil
}

func (r *DocumentRepo) Delete(ctx context.Context, docID string) error {
	_, err := r.ExecContext(ctx, `DELETE FROM documents WHERE doc_id=?`, docID)
	if err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	return nil
}

func (r *DocumentRepo) ListExpired(ctx context.Context, before time.Time) ([]*domain.Document, error) {
	rows, err := r.QueryContext(ctx, `SELECT doc_id, user_id, filename, content_type, size, checksum, storage_path, cleanup_at, created_at
		FROM documents WHERE cleanup_at IS NOT NULL AND cleanup_at < ?`, before)
	if err != nil {
		return nil, fmt.Errorf("list expired: %w", err)
	}
	defer rows.Close()
	var out []*domain.Document
	for rows.Next() {
		d := &domain.Document{}
		var contentType sql.NullString
		var cleanup sql.NullTime
		if err := rows.Scan(&d.DocID, &d.UserID, &d.Filename, &contentType, &d.Size, &d.Checksum, &d.StoragePath, &cleanup, &d.CreatedAt); err != nil {
			return nil, err
		}
		d.ContentType = contentType.String
		if cleanup.Valid {
			d.CleanupAt = cleanup.Time
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *DocumentRepo) ListByUser(ctx context.Context, userID string, limit, offset int) ([]*domain.Document, error) {
	rows, err := r.QueryContext(ctx, `SELECT doc_id, user_id, filename, content_type, size, checksum, storage_path, cleanup_at, created_at
		FROM documents WHERE user_id=? ORDER BY created_at DESC LIMIT ? OFFSET ?`, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list by user: %w", err)
	}
	defer rows.Close()
	var out []*domain.Document
	for rows.Next() {
		d := &domain.Document{}
		var contentType sql.NullString
		var cleanup sql.NullTime
		if err := rows.Scan(&d.DocID, &d.UserID, &d.Filename, &contentType, &d.Size, &d.Checksum, &d.StoragePath, &cleanup, &d.CreatedAt); err != nil {
			return nil, err
		}
		d.ContentType = contentType.String
		if cleanup.Valid {
			d.CleanupAt = cleanup.Time
		}
		out = append(out, d)
	}
	return out, rows.Err()
}