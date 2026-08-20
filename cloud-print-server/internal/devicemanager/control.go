package devicemanager

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/cloud-print/server/internal/domain"
	"github.com/cloud-print/server/internal/errs"
)

type CommandDispatcher interface {
	SendControl(agentID string, control domain.ControlPayload) error
}

type Controller struct {
	manager   *Manager
	dispatcher CommandDispatcher
	logger    *zap.Logger
}

func NewController(manager *Manager, dispatcher CommandDispatcher, logger *zap.Logger) *Controller {
	return &Controller{
		manager:    manager,
		dispatcher: dispatcher,
		logger:     logger,
	}
}

func (c *Controller) SetDispatcher(d CommandDispatcher) { c.dispatcher = d }

func (c *Controller) SendTestPage(ctx context.Context, deviceID string) error {
	d, err := c.manager.GetDevice(ctx, deviceID)
	if err != nil {
		return err
	}
	if d.Status != domain.DeviceStatusOnline {
		return errs.New(errs.ErrDeviceOffline, fmt.Sprintf("device %s is %s", deviceID, d.Status.String()))
	}
	if c.dispatcher == nil {
		return errs.New(errs.ErrInternalError, "dispatcher not configured")
	}
	ctrl := domain.ControlPayload{Action: "test_page", DeviceID: deviceID}
	if err := c.dispatcher.SendControl(d.AgentID, ctrl); err != nil {
		return errs.Wrap(errs.ErrAgentOffline, "send test page", err)
	}
	c.logger.Info("test page sent", zap.String("device_id", deviceID), zap.String("agent_id", d.AgentID))
	return nil
}

func (c *Controller) PauseDevice(ctx context.Context, deviceID string) error {
	d, err := c.manager.GetDevice(ctx, deviceID)
	if err != nil {
		return err
	}
	if c.dispatcher == nil {
		return errs.New(errs.ErrInternalError, "dispatcher not configured")
	}
	ctrl := domain.ControlPayload{Action: "pause", DeviceID: deviceID}
	if err := c.dispatcher.SendControl(d.AgentID, ctrl); err != nil {
		return errs.Wrap(errs.ErrAgentOffline, "send pause", err)
	}
	c.logger.Info("pause device sent", zap.String("device_id", deviceID))
	return nil
}

func (c *Controller) ResumeDevice(ctx context.Context, deviceID string) error {
	d, err := c.manager.GetDevice(ctx, deviceID)
	if err != nil {
		return err
	}
	if c.dispatcher == nil {
		return errs.New(errs.ErrInternalError, "dispatcher not configured")
	}
	ctrl := domain.ControlPayload{Action: "resume", DeviceID: deviceID}
	if err := c.dispatcher.SendControl(d.AgentID, ctrl); err != nil {
		return errs.Wrap(errs.ErrAgentOffline, "send resume", err)
	}
	c.logger.Info("resume device sent", zap.String("device_id", deviceID))
	return nil
}

func (c *Controller) SendControl(ctx context.Context, deviceID, action string) error {
	d, err := c.manager.GetDevice(ctx, deviceID)
	if err != nil {
		return err
	}
	if c.dispatcher == nil {
		return errs.New(errs.ErrInternalError, "dispatcher not configured")
	}
	ctrl := domain.ControlPayload{Action: action, DeviceID: deviceID}
	if err := c.dispatcher.SendControl(d.AgentID, ctrl); err != nil {
		return errs.Wrap(errs.ErrAgentOffline, "send control", err)
	}
	return nil
}