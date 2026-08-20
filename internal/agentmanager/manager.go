package agentmanager

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
	"nhooyr.io/websocket"

	"github.com/cloud-print/server/internal/domain"
)

type AgentConnection struct {
	AgentID       string
	Conn          *websocket.Conn
	LastHeartbeat time.Time
	OnlineDevices int
	PendingTasks  int
	NetClass      domain.NetClass
	CloudEndpoint string
	ConnectedAt   time.Time
	Version       string

	sendCh    chan []byte
	closeCh   chan struct{}
	closeOnce sync.Once
}

func (c *AgentConnection) SendCh() chan []byte  { return c.sendCh }
func (c *AgentConnection) CloseCh() chan struct{} { return c.closeCh }

func (c *AgentConnection) Close() {
	c.closeOnce.Do(func() {
		close(c.closeCh)
		_ = c.Conn.Close(websocket.StatusNormalClosure, "close")
	})
}

type Manager struct {
	mu     sync.RWMutex
	conns  map[string]*AgentConnection
	repo   *Repo
	cred   *CredentialManager
	logger *zap.Logger
}

func NewManager(repo *Repo, cred *CredentialManager, logger *zap.Logger) *Manager {
	return &Manager{
		conns:  make(map[string]*AgentConnection),
		repo:   repo,
		cred:   cred,
		logger: logger,
	}
}

func (m *Manager) Register(agentID string, conn *websocket.Conn) *AgentConnection {
	m.mu.Lock()
	if old, ok := m.conns[agentID]; ok {
		old.Close()
		delete(m.conns, agentID)
	}
	now := time.Now().UTC()
	c := &AgentConnection{
		AgentID:       agentID,
		Conn:          conn,
		LastHeartbeat: now,
		ConnectedAt:   now,
		sendCh:        make(chan []byte, 64),
		closeCh:       make(chan struct{}),
	}
	m.conns[agentID] = c
	m.mu.Unlock()

	if m.repo != nil {
		_ = m.repo.SetOnline(context.Background(), agentID, true)
	}
	return c
}

func (m *Manager) Unregister(agentID string) {
	m.mu.Lock()
	c, ok := m.conns[agentID]
	if ok {
		c.Close()
		delete(m.conns, agentID)
	}
	m.mu.Unlock()
	if ok && m.repo != nil {
		_ = m.repo.SetOnline(context.Background(), agentID, false)
	}
}

func (m *Manager) Get(agentID string) (*AgentConnection, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.conns[agentID]
	return c, ok
}

func (m *Manager) List() []*AgentConnection {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*AgentConnection, 0, len(m.conns))
	for _, c := range m.conns {
		out = append(out, c)
	}
	return out
}

func (m *Manager) UpdateHeartbeat(agentID string, payload domain.HeartbeatPayload) {
	m.mu.Lock()
	c, ok := m.conns[agentID]
	if ok {
		now := time.Now().UTC()
		c.LastHeartbeat = now
		c.OnlineDevices = payload.OnlineDevices
		c.PendingTasks = payload.PendingTasks
		c.NetClass = payload.NetClass
		c.CloudEndpoint = payload.CloudEndpoint
		c.Version = payload.Version
	}
	m.mu.Unlock()

	if ok && m.repo != nil {
		_ = m.repo.UpdateHeartbeat(context.Background(), agentID, payload.Version, payload.OnlineDevices, payload.PendingTasks, payload.NetClass)
	}
}

func (m *Manager) IsOnline(agentID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.conns[agentID]
	return ok
}

func (m *Manager) OnlineCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.conns)
}