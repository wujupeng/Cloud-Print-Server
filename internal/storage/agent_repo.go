package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/cloud-print/server/internal/domain"
)

type AgentRepo struct {
	*Repository
}

func NewAgentRepo(db *DB) *AgentRepo {
	return &AgentRepo{Repository: NewRepository(db)}
}

func (r *AgentRepo) Create(ctx context.Context, a *domain.Agent) error {
	_, err := r.ExecContext(ctx, `INSERT INTO agents
		(agent_id, name, factory_id, device_token_enc, version, online, online_devices, pending_tasks, net_class, last_heartbeat_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.AgentID, a.Name, a.FactoryID, a.DeviceTokenEnc, a.Version, boolToInt(a.Online), a.OnlineDevices, a.PendingTasks, a.NetClass, a.LastHeartbeatAt, time.Now().UTC(), time.Now().UTC())
	if err != nil {
		return fmt.Errorf("insert agent: %w", err)
	}
	return nil
}

func (r *AgentRepo) GetByID(ctx context.Context, agentID string) (*domain.Agent, error) {
	a := &domain.Agent{}
	var lastHB sql.NullTime
	err := r.QueryRowContext(ctx, `SELECT agent_id, name, factory_id, device_token_enc, version, online, online_devices, pending_tasks, net_class, last_heartbeat_at, created_at, updated_at
		FROM agents WHERE agent_id = ?`, agentID).Scan(
		&a.AgentID, &a.Name, &a.FactoryID, &a.DeviceTokenEnc, &a.Version, &a.Online, &a.OnlineDevices, &a.PendingTasks, &a.NetClass, &lastHB, &a.CreatedAt, &a.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}
	if lastHB.Valid {
		a.LastHeartbeatAt = lastHB.Time
	}
	return a, nil
}

func (r *AgentRepo) Update(ctx context.Context, a *domain.Agent) error {
	_, err := r.ExecContext(ctx, `UPDATE agents SET name=?, factory_id=?, device_token_enc=?, version=?, online=?, online_devices=?, pending_tasks=?, net_class=?, last_heartbeat_at=?, updated_at=?
		WHERE agent_id=?`,
		a.Name, a.FactoryID, a.DeviceTokenEnc, a.Version, boolToInt(a.Online), a.OnlineDevices, a.PendingTasks, a.NetClass, a.LastHeartbeatAt, time.Now().UTC(), a.AgentID)
	if err != nil {
		return fmt.Errorf("update agent: %w", err)
	}
	return nil
}

func (r *AgentRepo) UpdateHeartbeat(ctx context.Context, agentID, version string, onlineDevices, pendingTasks int, netClass domain.NetClass) error {
	_, err := r.ExecContext(ctx, `UPDATE agents SET version=?, online_devices=?, pending_tasks=?, net_class=?, last_heartbeat_at=?, updated_at=?
		WHERE agent_id=?`, version, onlineDevices, pendingTasks, netClass, time.Now().UTC(), time.Now().UTC(), agentID)
	if err != nil {
		return fmt.Errorf("update heartbeat: %w", err)
	}
	return nil
}

func (r *AgentRepo) SetOnline(ctx context.Context, agentID string, online bool) error {
	_, err := r.ExecContext(ctx, `UPDATE agents SET online=?, updated_at=? WHERE agent_id=?`,
		boolToInt(online), time.Now().UTC(), agentID)
	if err != nil {
		return fmt.Errorf("set online: %w", err)
	}
	return nil
}

func (r *AgentRepo) UpdateVersion(ctx context.Context, agentID, version string) error {
	_, err := r.ExecContext(ctx, `UPDATE agents SET version=?, updated_at=? WHERE agent_id=?`,
		version, time.Now().UTC(), agentID)
	if err != nil {
		return fmt.Errorf("update version: %w", err)
	}
	return nil
}

func (r *AgentRepo) UpdateNetClass(ctx context.Context, agentID string, netClass domain.NetClass) error {
	_, err := r.ExecContext(ctx, `UPDATE agents SET net_class=?, updated_at=? WHERE agent_id=?`,
		netClass, time.Now().UTC(), agentID)
	if err != nil {
		return fmt.Errorf("update net_class: %w", err)
	}
	return nil
}

func (r *AgentRepo) Delete(ctx context.Context, agentID string) error {
	_, err := r.ExecContext(ctx, `DELETE FROM agents WHERE agent_id=?`, agentID)
	if err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	return nil
}

func (r *AgentRepo) List(ctx context.Context) ([]*domain.Agent, error) {
	rows, err := r.QueryContext(ctx, `SELECT agent_id, name, factory_id, device_token_enc, version, online, online_devices, pending_tasks, net_class, last_heartbeat_at, created_at, updated_at
		FROM agents ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()
	var out []*domain.Agent
	for rows.Next() {
		a := &domain.Agent{}
		var lastHB sql.NullTime
		if err := rows.Scan(&a.AgentID, &a.Name, &a.FactoryID, &a.DeviceTokenEnc, &a.Version, &a.Online, &a.OnlineDevices, &a.PendingTasks, &a.NetClass, &lastHB, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		if lastHB.Valid {
			a.LastHeartbeatAt = lastHB.Time
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}