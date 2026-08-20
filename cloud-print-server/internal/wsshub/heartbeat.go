package wsshub

import (
	"context"
	"time"

	"go.uber.org/zap"
)

func (h *Hub) StartHeartbeatChecker(ctx context.Context) {
	ticker := time.NewTicker(h.checkInterval)
	defer ticker.Stop()
	h.logger.Info("heartbeat checker started",
		zap.Duration("timeout", h.heartbeatTimeout),
		zap.Duration("interval", h.checkInterval),
	)
	for {
		select {
		case <-ctx.Done():
			h.logger.Info("heartbeat checker stopped")
			return
		case <-ticker.C:
			h.checkHeartbeats()
		}
	}
}

func (h *Hub) checkHeartbeats() {
	now := time.Now().UTC()
	for _, ac := range h.agentMgr.List() {
		since := now.Sub(ac.LastHeartbeat)
		if since > h.heartbeatTimeout {
			h.logger.Warn("agent heartbeat timeout, marking offline",
				zap.String("agent_id", ac.AgentID),
				zap.Duration("since_heartbeat", since),
				zap.Duration("timeout", h.heartbeatTimeout),
			)
			h.agentMgr.Unregister(ac.AgentID)
		}
	}
}