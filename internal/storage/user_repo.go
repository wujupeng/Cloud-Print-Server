package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/cloud-print/server/internal/domain"
)

type UserRepo struct {
	*Repository
}

func NewUserRepo(db *DB) *UserRepo {
	return &UserRepo{Repository: NewRepository(db)}
}

func (r *UserRepo) Create(ctx context.Context, u *domain.User) error {
	_, err := r.ExecContext(ctx, `INSERT INTO users
		(user_id, username, password_hash, password_salt, role, status, display_name, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.UserID, u.Username, u.PasswordHash, u.PasswordSalt, u.Role, u.Status, u.DisplayName, time.Now().UTC(), time.Now().UTC())
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

func (r *UserRepo) GetByID(ctx context.Context, userID string) (*domain.User, error) {
	u := &domain.User{}
	err := r.QueryRowContext(ctx, `SELECT user_id, username, password_hash, password_salt, role, status, display_name, created_at, updated_at
		FROM users WHERE user_id = ?`, userID).Scan(
		&u.UserID, &u.Username, &u.PasswordHash, &u.PasswordSalt, &u.Role, &u.Status, &u.DisplayName, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return u, nil
}

func (r *UserRepo) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	u := &domain.User{}
	err := r.QueryRowContext(ctx, `SELECT user_id, username, password_hash, password_salt, role, status, display_name, created_at, updated_at
		FROM users WHERE username = ?`, username).Scan(
		&u.UserID, &u.Username, &u.PasswordHash, &u.PasswordSalt, &u.Role, &u.Status, &u.DisplayName, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by username: %w", err)
	}
	return u, nil
}

func (r *UserRepo) Update(ctx context.Context, u *domain.User) error {
	_, err := r.ExecContext(ctx, `UPDATE users SET username=?, password_hash=?, password_salt=?, role=?, status=?, display_name=?, updated_at=?
		WHERE user_id=?`,
		u.Username, u.PasswordHash, u.PasswordSalt, u.Role, u.Status, u.DisplayName, time.Now().UTC(), u.UserID)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}

func (r *UserRepo) Delete(ctx context.Context, userID string) error {
	_, err := r.ExecContext(ctx, `DELETE FROM users WHERE user_id=?`, userID)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

func (r *UserRepo) List(ctx context.Context) ([]*domain.User, error) {
	rows, err := r.QueryContext(ctx, `SELECT user_id, username, password_hash, password_salt, role, status, display_name, created_at, updated_at
		FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	var out []*domain.User
	for rows.Next() {
		u := &domain.User{}
		if err := rows.Scan(&u.UserID, &u.Username, &u.PasswordHash, &u.PasswordSalt, &u.Role, &u.Status, &u.DisplayName, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *UserRepo) ListPermissions(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.QueryContext(ctx, `SELECT device_id FROM user_permissions WHERE user_id=?`, userID)
	if err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *UserRepo) GrantPermission(ctx context.Context, userID, deviceID string) error {
	_, err := r.ExecContext(ctx, `INSERT OR IGNORE INTO user_permissions (permission_id, user_id, device_id, granted_at)
		VALUES (?, ?, ?, ?)`, userID+"_"+deviceID, userID, deviceID, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("grant permission: %w", err)
	}
	return nil
}

func (r *UserRepo) RevokePermission(ctx context.Context, userID, deviceID string) error {
	_, err := r.ExecContext(ctx, `DELETE FROM user_permissions WHERE user_id=? AND device_id=?`, userID, deviceID)
	if err != nil {
		return fmt.Errorf("revoke permission: %w", err)
	}
	return nil
}

func (r *UserRepo) HasPermission(ctx context.Context, userID, deviceID string) (bool, error) {
	var v string
	err := r.QueryRowContext(ctx, `SELECT permission_id FROM user_permissions WHERE user_id=? AND device_id=?`, userID, deviceID).Scan(&v)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}