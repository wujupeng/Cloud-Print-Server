package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/cloud-print/server/internal/domain"
)

type DeviceRepo struct {
	*Repository
}

func NewDeviceRepo(db *DB) *DeviceRepo {
	return &DeviceRepo{Repository: NewRepository(db)}
}

func (r *DeviceRepo) Create(ctx context.Context, d *domain.Device) error {
	now := time.Now().UTC()
	_, err := r.ExecContext(ctx, `INSERT INTO devices
		(device_id, name, ip, hostname, model, protocol, status, factory_id, agent_id, port, last_probe_at, last_status_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.DeviceID, d.Name, d.IP, d.Hostname, d.Model, d.Protocol, d.Status, d.FactoryID, d.AgentID, d.Port, d.LastProbeAt, d.LastStatusAt, now, now)
	if err != nil {
		return fmt.Errorf("insert device: %w", err)
	}
	return nil
}

func (r *DeviceRepo) GetByID(ctx context.Context, deviceID string) (*domain.Device, error) {
	d := &domain.Device{}
	var hostname sql.NullString
	var lastProbe, lastStatus sql.NullTime
	err := r.QueryRowContext(ctx, `SELECT device_id, name, ip, hostname, model, protocol, status, factory_id, agent_id, port, last_probe_at, last_status_at, created_at, updated_at
		FROM devices WHERE device_id = ?`, deviceID).Scan(
		&d.DeviceID, &d.Name, &d.IP, &hostname, &d.Model, &d.Protocol, &d.Status, &d.FactoryID, &d.AgentID, &d.Port, &lastProbe, &lastStatus, &d.CreatedAt, &d.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get device: %w", err)
	}
	if hostname.Valid {
		d.Hostname = hostname.String
	}
	if lastProbe.Valid {
		d.LastProbeAt = lastProbe.Time
	}
	if lastStatus.Valid {
		d.LastStatusAt = lastStatus.Time
	}
	return d, nil
}

func (r *DeviceRepo) Update(ctx context.Context, d *domain.Device) error {
	_, err := r.ExecContext(ctx, `UPDATE devices SET name=?, ip=?, hostname=?, model=?, protocol=?, status=?, factory_id=?, agent_id=?, port=?, last_probe_at=?, last_status_at=?, updated_at=?
		WHERE device_id=?`,
		d.Name, d.IP, d.Hostname, d.Model, d.Protocol, d.Status, d.FactoryID, d.AgentID, d.Port, d.LastProbeAt, d.LastStatusAt, time.Now().UTC(), d.DeviceID)
	if err != nil {
		return fmt.Errorf("update device: %w", err)
	}
	return nil
}

func (r *DeviceRepo) UpdateStatus(ctx context.Context, deviceID string, status domain.DeviceStatus, protocol domain.Protocol) error {
	_, err := r.ExecContext(ctx, `UPDATE devices SET status=?, protocol=?, last_status_at=?, updated_at=? WHERE device_id=?`,
		status, protocol, time.Now().UTC(), time.Now().UTC(), deviceID)
	if err != nil {
		return fmt.Errorf("update device status: %w", err)
	}
	return nil
}

func (r *DeviceRepo) SetStatusByAgent(ctx context.Context, agentID string, status domain.DeviceStatus) error {
	_, err := r.ExecContext(ctx, `UPDATE devices SET status=?, updated_at=? WHERE agent_id=?`,
		status, time.Now().UTC(), agentID)
	if err != nil {
		return fmt.Errorf("set status by agent: %w", err)
	}
	return nil
}

func (r *DeviceRepo) Delete(ctx context.Context, deviceID string) error {
	_, err := r.ExecContext(ctx, `DELETE FROM devices WHERE device_id=?`, deviceID)
	if err != nil {
		return fmt.Errorf("delete device: %w", err)
	}
	return nil
}

func (r *DeviceRepo) List(ctx context.Context) ([]*domain.Device, error) {
	return r.queryList(ctx, `SELECT device_id, name, ip, hostname, model, protocol, status, factory_id, agent_id, port, last_probe_at, last_status_at, created_at, updated_at
		FROM devices ORDER BY created_at DESC`)
}

func (r *DeviceRepo) ListByFactory(ctx context.Context, factoryID string) ([]*domain.Device, error) {
	return r.queryList(ctx, `SELECT device_id, name, ip, hostname, model, protocol, status, factory_id, agent_id, port, last_probe_at, last_status_at, created_at, updated_at
		FROM devices WHERE factory_id=? ORDER BY created_at DESC`, factoryID)
}

func (r *DeviceRepo) ListByAgent(ctx context.Context, agentID string) ([]*domain.Device, error) {
	return r.queryList(ctx, `SELECT device_id, name, ip, hostname, model, protocol, status, factory_id, agent_id, port, last_probe_at, last_status_at, created_at, updated_at
		FROM devices WHERE agent_id=? ORDER BY created_at DESC`, agentID)
}

func (r *DeviceRepo) queryList(ctx context.Context, query string, args ...interface{}) ([]*domain.Device, error) {
	rows, err := r.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Device
	for rows.Next() {
		d := &domain.Device{}
		var hostname sql.NullString
		var lastProbe, lastStatus sql.NullTime
		if err := rows.Scan(&d.DeviceID, &d.Name, &d.IP, &hostname, &d.Model, &d.Protocol, &d.Status, &d.FactoryID, &d.AgentID, &d.Port, &lastProbe, &lastStatus, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		if hostname.Valid {
			d.Hostname = hostname.String
		}
		if lastProbe.Valid {
			d.LastProbeAt = lastProbe.Time
		}
		if lastStatus.Valid {
			d.LastStatusAt = lastStatus.Time
		}
		out = append(out, d)
	}
	return out, rows.Err()
}