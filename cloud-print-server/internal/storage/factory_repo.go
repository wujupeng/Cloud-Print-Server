package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/cloud-print/server/internal/domain"
)

type FactoryRepo struct {
	*Repository
}

func NewFactoryRepo(db *DB) *FactoryRepo {
	return &FactoryRepo{Repository: NewRepository(db)}
}

func (r *FactoryRepo) Create(ctx context.Context, f *domain.Factory) error {
	now := time.Now().UTC()
	_, err := r.ExecContext(ctx, `INSERT INTO factories (factory_id, name, code, location, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`, f.FactoryID, f.Name, f.Code, f.Location, now, now)
	if err != nil {
		return fmt.Errorf("insert factory: %w", err)
	}
	return nil
}

func (r *FactoryRepo) GetByID(ctx context.Context, factoryID string) (*domain.Factory, error) {
	f := &domain.Factory{}
	var code, location sql.NullString
	err := r.QueryRowContext(ctx, `SELECT factory_id, name, code, location, created_at, updated_at FROM factories WHERE factory_id=?`, factoryID).
		Scan(&f.FactoryID, &f.Name, &code, &location, &f.CreatedAt, &f.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get factory: %w", err)
	}
	f.Code = code.String
	f.Location = location.String
	return f, nil
}

func (r *FactoryRepo) Update(ctx context.Context, f *domain.Factory) error {
	_, err := r.ExecContext(ctx, `UPDATE factories SET name=?, code=?, location=?, updated_at=? WHERE factory_id=?`,
		f.Name, f.Code, f.Location, time.Now().UTC(), f.FactoryID)
	if err != nil {
		return fmt.Errorf("update factory: %w", err)
	}
	return nil
}

func (r *FactoryRepo) Delete(ctx context.Context, factoryID string) error {
	_, err := r.ExecContext(ctx, `DELETE FROM factories WHERE factory_id=?`, factoryID)
	if err != nil {
		return fmt.Errorf("delete factory: %w", err)
	}
	return nil
}

func (r *FactoryRepo) List(ctx context.Context) ([]*domain.Factory, error) {
	rows, err := r.QueryContext(ctx, `SELECT factory_id, name, code, location, created_at, updated_at FROM factories ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list factories: %w", err)
	}
	defer rows.Close()
	var out []*domain.Factory
	for rows.Next() {
		f := &domain.Factory{}
		var code, location sql.NullString
		if err := rows.Scan(&f.FactoryID, &f.Name, &code, &location, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		f.Code = code.String
		f.Location = location.String
		out = append(out, f)
	}
	return out, rows.Err()
}

func (r *FactoryRepo) HasDependents(ctx context.Context, factoryID string) (bool, error) {
	var c int
	if err := r.QueryRowContext(ctx, `SELECT COUNT(*) FROM agents WHERE factory_id=?`, factoryID).Scan(&c); err != nil {
		return false, err
	}
	if c > 0 {
		return true, nil
	}
	if err := r.QueryRowContext(ctx, `SELECT COUNT(*) FROM devices WHERE factory_id=?`, factoryID).Scan(&c); err != nil {
		return false, err
	}
	return c > 0, nil
}