package devicemanager

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/cloud-print/server/internal/domain"
	"github.com/cloud-print/server/internal/errs"
	"github.com/cloud-print/server/internal/storage"
)

type Manager struct {
	deviceRepo *storage.DeviceRepo
	userRepo   *storage.UserRepo
	logger     *zap.Logger
}

func NewManager(deviceRepo *storage.DeviceRepo, logger *zap.Logger) *Manager {
	return &Manager{
		deviceRepo: deviceRepo,
		logger:     logger,
	}
}

func (m *Manager) SetUserRepo(userRepo *storage.UserRepo) { m.userRepo = userRepo }

func (m *Manager) ListDevices(ctx context.Context, userID string) ([]*domain.Device, error) {
	all, err := m.deviceRepo.List(ctx)
	if err != nil {
		return nil, errs.Wrap(errs.ErrInternalError, "list devices", err)
	}
	if m.userRepo == nil {
		return all, nil
	}
	allowed, err := m.userRepo.ListPermissions(ctx, userID)
	if err != nil {
		return nil, errs.Wrap(errs.ErrInternalError, "list permissions", err)
	}
	if len(allowed) == 0 {
		return []*domain.Device{}, nil
	}
	allowSet := make(map[string]struct{}, len(allowed))
	for _, d := range allowed {
		allowSet[d] = struct{}{}
	}
	out := make([]*domain.Device, 0, len(all))
	for _, d := range all {
		if _, ok := allowSet[d.DeviceID]; ok {
			out = append(out, d)
		}
	}
	return out, nil
}

func (m *Manager) ListDevicesByFactory(ctx context.Context, factoryID string) ([]*domain.Device, error) {
	return m.deviceRepo.ListByFactory(ctx, factoryID)
}

func (m *Manager) ListDevicesByAgent(ctx context.Context, agentID string) ([]*domain.Device, error) {
	return m.deviceRepo.ListByAgent(ctx, agentID)
}

func (m *Manager) GetDevice(ctx context.Context, deviceID string) (*domain.Device, error) {
	d, err := m.deviceRepo.GetByID(ctx, deviceID)
	if err != nil {
		return nil, errs.Wrap(errs.ErrInternalError, "get device", err)
	}
	if d == nil {
		return nil, errs.New(errs.ErrDeviceNotFound, fmt.Sprintf("device %s not found", deviceID))
	}
	return d, nil
}

func (m *Manager) HandleDeviceStatus(ctx context.Context, agentID, deviceID string, status domain.DeviceStatus, protocol domain.Protocol) error {
	d, err := m.deviceRepo.GetByID(ctx, deviceID)
	if err != nil {
		return errs.Wrap(errs.ErrInternalError, "get device", err)
	}
	if d == nil {
		m.logger.Warn("device status for unknown device",
			zap.String("agent_id", agentID),
			zap.String("device_id", deviceID),
		)
		return errs.New(errs.ErrDeviceNotFound, fmt.Sprintf("device %s not found", deviceID))
	}
	if d.AgentID != agentID {
		m.logger.Warn("device status agent mismatch",
			zap.String("device_id", deviceID),
			zap.String("expected_agent", d.AgentID),
			zap.String("actual_agent", agentID),
		)
	}
	if err := m.deviceRepo.UpdateStatus(ctx, deviceID, status, protocol); err != nil {
		return errs.Wrap(errs.ErrInternalError, "update device status", err)
	}
	m.logger.Info("device status updated",
		zap.String("device_id", deviceID),
		zap.String("agent_id", agentID),
		zap.String("status", status.String()),
		zap.String("protocol", protocol.String()),
	)
	return nil
}

func (m *Manager) CreateDevice(ctx context.Context, d *domain.Device) error {
	if d.DeviceID == "" {
		return errs.New(errs.ErrParamInvalid, "device_id is required")
	}
	d.LastStatusAt = time.Now().UTC()
	return m.deviceRepo.Create(ctx, d)
}

func (m *Manager) UpdateDevice(ctx context.Context, d *domain.Device) error {
	return m.deviceRepo.Update(ctx, d)
}

func (m *Manager) DeleteDevice(ctx context.Context, deviceID string) error {
	return m.deviceRepo.Delete(ctx, deviceID)
}