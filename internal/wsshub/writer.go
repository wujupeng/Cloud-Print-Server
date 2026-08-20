package wsshub

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"
	"nhooyr.io/websocket"

	"github.com/cloud-print/server/internal/agentmanager"
	"github.com/cloud-print/server/internal/domain"
	"github.com/cloud-print/server/internal/errs"
	"github.com/google/uuid"
)

func (h *Hub) writeLoop(ctx context.Context, ac *agentmanager.AgentConnection) {
	defer ac.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ac.CloseCh():
			return
		case data, ok := <-ac.SendCh():
			if !ok {
				return
			}
			writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := ac.Conn.Write(writeCtx, websocket.MessageText, data)
			cancel()
			if err != nil {
				h.logger.Warn("write loop exit", zap.String("agent_id", ac.AgentID), zap.Error(err))
				return
			}
		}
	}
}

func (h *Hub) SendToAgent(agentID string, env *domain.DownEnvelope) error {
	ac, ok := h.agentMgr.Get(agentID)
	if !ok {
		return errs.New(errs.ErrAgentOffline, "agent not online")
	}
	data, err := json.Marshal(env)
	if err != nil {
		return errs.Wrap(errs.ErrInternalError, "marshal envelope", err)
	}
	select {
	case ac.SendCh() <- data:
		return nil
	default:
		return errs.New(errs.ErrQueueFull, "send queue full")
	}
}

func (h *Hub) Broadcast(env *domain.DownEnvelope) {
	data, err := json.Marshal(env)
	if err != nil {
		h.logger.Error("broadcast marshal failed", zap.Error(err))
		return
	}
	for _, ac := range h.agentMgr.List() {
		select {
		case ac.SendCh() <- data:
		default:
			h.logger.Warn("broadcast drop, queue full", zap.String("agent_id", ac.AgentID))
		}
	}
}

func (h *Hub) SendTask(agentID string, task *domain.PrintTask) error {
	p := domain.TaskPayload{
		TaskID:   task.TaskID,
		DeviceID: task.DeviceID,
		Data:     task.DocumentRef,
		Checksum: task.Checksum,
		Params:   task.Params,
	}
	env, err := domain.NewDownEnvelope(domain.MsgTask, uuid.NewString(), task.TraceID, p)
	if err != nil {
		return err
	}
	return h.SendToAgent(agentID, env)
}

func (h *Hub) SendControl(agentID string, control domain.ControlPayload) error {
	env, err := domain.NewDownEnvelope(domain.MsgControl, uuid.NewString(), "", control)
	if err != nil {
		return err
	}
	return h.SendToAgent(agentID, env)
}

func (h *Hub) SendConfigUpdate(agentID string, cfg domain.ConfigUpdatePayload) error {
	env, err := domain.NewDownEnvelope(domain.MsgConfigUpdate, uuid.NewString(), "", cfg)
	if err != nil {
		return err
	}
	return h.SendToAgent(agentID, env)
}

func (h *Hub) SendUpgrade(agentID string, up domain.UpgradePayload) error {
	env, err := domain.NewDownEnvelope(domain.MsgUpgrade, uuid.NewString(), "", up)
	if err != nil {
		return err
	}
	return h.SendToAgent(agentID, env)
}