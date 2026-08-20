package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/cloud-print/server/internal/domain"
)

type AuditRepo struct {
	*Repository
}

func NewAuditRepo(db *DB) *AuditRepo {
	return &AuditRepo{Repository: NewRepository(db)}
}

func (r *AuditRepo) Create(ctx context.Context, a *domain.AuditLog) error {
	if a.TS.IsZero() {
		a.TS = time.Now().UTC()
	}
	_, err := r.ExecContext(ctx, `INSERT INTO audit_logs (audit_id, user_id, action, target, detail, ip, ts)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, a.AuditID, a.UserID, a.Action, a.Target, a.Detail, a.IP, a.TS)
	if err != nil {
		return fmt.Errorf("insert audit: %w", err)
	}
	return nil
}

func (r *AuditRepo) List(ctx context.Context, action string, start, end time.Time, limit, offset int) ([]*domain.AuditLog, error) {
	q := `SELECT audit_id, user_id, action, target, detail, ip, ts FROM audit_logs WHERE 1=1`
	args := []interface{}{}
	if action != "" {
		q += ` AND action=?`
		args = append(args, action)
	}
	if !start.IsZero() {
		q += ` AND ts>=?`
		args = append(args, start)
	}
	if !end.IsZero() {
		q += ` AND ts<=?`
		args = append(args, end)
	}
	q += ` ORDER BY ts DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := r.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list audit: %w", err)
	}
	defer rows.Close()
	var out []*domain.AuditLog
	for rows.Next() {
		a := &domain.AuditLog{}
		var userID, target, detail, ip sql.NullString
		if err := rows.Scan(&a.AuditID, &userID, &a.Action, &target, &detail, &ip, &a.TS); err != nil {
			return nil, err
		}
		a.UserID = userID.String
		a.Target = target.String
		a.Detail = detail.String
		a.IP = ip.String
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *AuditRepo) Cleanup(ctx context.Context, before time.Time) error {
	_, err := r.ExecContext(ctx, `DELETE FROM audit_logs WHERE ts < ?`, before)
	if err != nil {
		return fmt.Errorf("cleanup audit: %w", err)
	}
	return nil
}