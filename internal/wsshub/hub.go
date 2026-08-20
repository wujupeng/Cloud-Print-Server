package wsshub

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"go.uber.org/zap"
	"nhooyr.io/websocket"

	"github.com/cloud-print/server/internal/agentmanager"
	"github.com/cloud-print/server/internal/domain"
	"github.com/cloud-print/server/internal/errs"
	"github.com/cloud-print/server/internal/observability"
	"github.com/cloud-print/server/internal/taskmanager"
)

type Hub struct {
	agentMgr       *agentmanager.Manager
	taskMgr        *taskmanager.TaskManager
	logger         *zap.Logger
	audit          *observability.AuditLogger
	heartbeatTimeout time.Duration
	checkInterval   time.Duration
}

func NewHub(agentMgr *agentmanager.Manager, taskMgr *taskmanager.TaskManager, logger *zap.Logger, audit *observability.AuditLogger) *Hub {
	return &Hub{
		agentMgr:         agentMgr,
		taskMgr:          taskMgr,
		logger:           logger,
		audit:            audit,
		heartbeatTimeout: 90 * time.Second,
		checkInterval:    15 * time.Second,
	}
}

func (h *Hub) SetHeartbeatTimeout(d time.Duration) { h.heartbeatTimeout = d }
func (h *Hub) SetCheckInterval(d time.Duration)    { h.checkInterval = d }

func (h *Hub) HandleAgentWS(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")
	token := r.URL.Query().Get("token")
	if agentID == "" || token == "" {
		writeWSSReject(w, http.StatusUnauthorized, "missing agent_id or token")
		return
	}
	if !h.agentMgr.ValidateAgentToken(r.Context(), agentID, token) {
		writeWSSReject(w, http.StatusUnauthorized, "invalid agent token")
		h.logger.Warn("agent auth failed", zap.String("agent_id", agentID), zap.String("remote", r.RemoteAddr))
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols:       []string{"v1"},
		InsecureSkipVerify: false,
	})
	if err != nil {
		h.logger.Warn("wss accept failed", zap.String("agent_id", agentID), zap.Error(err))
		return
	}
	conn.SetReadLimit(1 << 20)

	hsCtx, hsCancel := context.WithTimeout(r.Context(), 10*time.Second)
	hsEnv, _ := domain.NewEnvelope("auth_ok", map[string]string{"status": "ok"})
	hsData, _ := json.Marshal(hsEnv)
	if err := conn.Write(hsCtx, websocket.MessageText, hsData); err != nil {
		h.logger.Warn("send handshake failed", zap.String("agent_id", agentID), zap.Error(err))
		_ = conn.Close(websocket.StatusInternalError, "handshake failed")
		hsCancel()
		return
	}
	hsCancel()

	ac := h.agentMgr.Register(agentID, conn)
	h.logger.Info("agent connected", zap.String("agent_id", agentID), zap.String("remote", r.RemoteAddr))

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go h.readLoop(ctx, ac)
	h.writeLoop(ctx, ac)

	h.agentMgr.Unregister(agentID)
	h.logger.Info("agent disconnected", zap.String("agent_id", agentID))
}

func writeWSSReject(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": string(errs.ErrAuthInvalid), "message": msg})
}