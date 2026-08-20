package wsshub

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"
	"nhooyr.io/websocket"

	"github.com/cloud-print/server/internal/agentmanager"
	"github.com/cloud-print/server/internal/domain"
)

func (h *Hub) readLoop(ctx context.Context, ac *agentmanager.AgentConnection) {
	defer ac.Close()
	for {
		_, data, err := ac.Conn.Read(ctx)
		if err != nil {
			h.logger.Info("read loop exit", zap.String("agent_id", ac.AgentID), zap.Error(err))
			return
		}
		var env domain.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			h.logger.Warn("invalid envelope", zap.String("agent_id", ac.AgentID), zap.Error(err))
			continue
		}
		if err := h.HandleMessage(ctx, ac.AgentID, &env); err != nil {
			h.logger.Warn("handle message failed",
				zap.String("agent_id", ac.AgentID),
				zap.String("type", env.Type),
				zap.Error(err),
			)
		}
	}
}

func (h *Hub) HandleMessage(ctx context.Context, agentID string, env *domain.Envelope) error {
	if !domain.IsUpstream(env.Type) {
		h.logger.Warn("unknown upstream type", zap.String("agent_id", agentID), zap.String("type", env.Type))
		return nil
	}
	switch env.Type {
	case domain.MsgHeartbeat:
		return h.handleHeartbeat(ctx, agentID, env)
	case domain.MsgTaskAck:
		return h.handleTaskAck(ctx, agentID, env)
	case domain.MsgTaskResult:
		return h.handleTaskResult(ctx, agentID, env)
	case domain.MsgDeviceStatus:
		return h.handleDeviceStatus(ctx, agentID, env)
	case domain.MsgNetEvent:
		return h.handleNetEvent(ctx, agentID, env)
	case domain.MsgConfigAck:
		return h.handleConfigAck(ctx, agentID, env)
	case domain.MsgLog:
		return h.handleLog(ctx, agentID, env)
	case domain.MsgUpgradeResult:
		return h.handleUpgradeResult(ctx, agentID, env)
	}
	return nil
}

func (h *Hub) handleHeartbeat(ctx context.Context, agentID string, env *domain.Envelope) error {
	var p domain.HeartbeatPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return err
	}
	h.agentMgr.UpdateHeartbeat(agentID, p)

	ackEnv, _ := domain.NewEnvelope("heartbeat_ack", map[string]string{"status": "ok"})
	ackData, _ := json.Marshal(ackEnv)
	ac, ok := h.agentMgr.Get(agentID)
	if ok {
		writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := ac.Conn.Write(writeCtx, websocket.MessageText, ackData)
		cancel()
		if err != nil {
			h.logger.Warn("send heartbeat ack failed", zap.String("agent_id", agentID), zap.Error(err))
		}
	}
	return nil
}

func (h *Hub) handleTaskAck(ctx context.Context, agentID string, env *domain.Envelope) error {
	var p domain.TaskAckPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return err
	}
	return h.taskMgr.HandleAck(ctx, p)
}

func (h *Hub) handleTaskResult(ctx context.Context, agentID string, env *domain.Envelope) error {
	var p domain.TaskResultPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return err
	}
	return h.taskMgr.HandleResult(ctx, p)
}

func (h *Hub) handleDeviceStatus(ctx context.Context, agentID string, env *domain.Envelope) error {
	var p domain.DeviceStatusPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return err
	}
	h.logger.Info("device status update",
		zap.String("agent_id", agentID),
		zap.String("device_id", p.DeviceID),
		zap.String("status", p.Status.String()),
	)
	return nil
}

func (h *Hub) handleNetEvent(ctx context.Context, agentID string, env *domain.Envelope) error {
	var p domain.NetEventPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return err
	}
	h.logger.Info("net event",
		zap.String("agent_id", agentID),
		zap.String("class", p.Class.String()),
		zap.String("endpoint", p.Endpoint),
		zap.String("detail", p.Detail),
	)
	return nil
}

func (h *Hub) handleConfigAck(ctx context.Context, agentID string, env *domain.Envelope) error {
	var p domain.ConfigAckPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return err
	}
	h.logger.Info("config ack",
		zap.String("agent_id", agentID),
		zap.Bool("applied", p.Applied),
		zap.String("field", p.Field),
		zap.String("reason", p.Reason),
	)
	return nil
}

func (h *Hub) handleLog(ctx context.Context, agentID string, env *domain.Envelope) error {
	var p domain.LogPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return err
	}
	h.logger.Info("agent log",
		zap.String("agent_id", agentID),
		zap.String("level", p.Level),
		zap.String("event", p.Event),
		zap.String("message", p.Message),
	)
	return nil
}

func (h *Hub) handleUpgradeResult(ctx context.Context, agentID string, env *domain.Envelope) error {
	var p domain.UpgradeResultPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return err
	}
	h.logger.Info("upgrade result",
		zap.String("agent_id", agentID),
		zap.Bool("success", p.Success),
		zap.String("from_ver", p.FromVer),
		zap.String("to_ver", p.ToVer),
		zap.Bool("rollback", p.Rollback),
	)
	return nil
}