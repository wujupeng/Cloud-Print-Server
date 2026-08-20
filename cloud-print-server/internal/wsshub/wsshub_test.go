package wsshub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"nhooyr.io/websocket"

	"github.com/cloud-print/server/internal/agentmanager"
	"github.com/cloud-print/server/internal/domain"
	"github.com/cloud-print/server/internal/docstore"
	"github.com/cloud-print/server/internal/observability"
	"github.com/cloud-print/server/internal/storage"
	"github.com/cloud-print/server/internal/taskmanager"
)

func setupTestHub(t *testing.T) (*Hub, *agentmanager.Manager, *taskmanager.TaskManager, *storage.DB, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	migrationsDir := resolveMigrationsDirW(t)

	err := storage.RunMigrations(dbPath, migrationsDir)
	require.NoError(t, err)

	db, err := storage.Open(dbPath)
	require.NoError(t, err)

	logger := zap.NewNop()
	audit := observability.NewAuditLoggerWrapper(logger)

	agentRepo := storage.NewAgentRepo(db)


	agentMgrRepo := agentmanager.NewRepo(agentRepo)
	credMgr := agentmanager.NewCredentialManager("test-master-key")
	agentMgr := agentmanager.NewManager(agentMgrRepo, credMgr, logger)

	docStore := docstore.NewStore(filepath.Join(dir, "docs"), logger)
	taskRepo := storage.NewTaskRepo(db)
	taskMgr := taskmanager.NewTaskManager(taskRepo, docStore, nil, logger, audit)
	deviceRepo := storage.NewDeviceRepo(db)
	taskMgr.SetDeviceRepo(deviceRepo)

	hub := NewHub(agentMgr, taskMgr, logger, audit)
	hub.SetHeartbeatTimeout(200 * time.Millisecond)
	hub.SetCheckInterval(50 * time.Millisecond)

	cleanup := func() {
		_ = db.Close()
		_ = os.RemoveAll(dir)
	}
	return hub, agentMgr, taskMgr, db, cleanup
}

func resolveMigrationsDirW(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"../../migrations",
		"../../../migrations",
		"./migrations",
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	t.Skip("migrations 目录未找到，跳过")
	return ""
}

func newEnvelope(t *testing.T, msgType string, payload interface{}) *domain.Envelope {
	t.Helper()
	b, err := json.Marshal(payload)
	require.NoError(t, err)
	return &domain.Envelope{Type: msgType, TS: time.Now().UTC(), Payload: b}
}

func newWSSPair(t *testing.T) (*websocket.Conn, *websocket.Conn, func()) {
	t.Helper()
	serverConnCh := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		serverConnCh <- conn
		<-r.Context().Done()
	}))

	dialCtx, dialCancel := context.WithTimeout(context.Background(), 5*time.Second)
	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	clientConn, _, err := websocket.Dial(dialCtx, url, &websocket.DialOptions{
		HTTPHeader: http.Header{},
	})
	dialCancel()
	require.NoError(t, err)

	var serverConn *websocket.Conn
	select {
	case serverConn = <-serverConnCh:
	case <-time.After(2 * time.Second):
		t.Fatal("等待服务端连接超时")
	}

	cleanup := func() {
		_ = clientConn.Close(websocket.StatusNormalClosure, "")
		_ = serverConn.Close(websocket.StatusNormalClosure, "")
		srv.Close()
	}
	return serverConn, clientConn, cleanup
}

func TestHubHandleMessageHeartbeat(t *testing.T) {
	hub, agentMgr, _, _, cleanup := setupTestHub(t)
	defer cleanup()

	serverConn, _, connCleanup := newWSSPair(t)
	defer connCleanup()
	ac := agentMgr.Register("agent-1", serverConn)
	defer agentMgr.Unregister("agent-1")
	_ = ac

	env := newEnvelope(t, domain.MsgHeartbeat, domain.HeartbeatPayload{
		AgentID:       "agent-1",
		Version:       "0.2.0",
		OnlineDevices: 3,
		PendingTasks:  5,
		CloudEndpoint: "print.oascii.com",
		NetClass:      domain.NetClassOK,
		Timestamp:     time.Now().UTC(),
	})

	err := hub.HandleMessage(context.Background(), "agent-1", env)
	require.NoError(t, err)

	got, ok := agentMgr.Get("agent-1")
	require.True(t, ok)
	assert.Equal(t, "0.2.0", got.Version)
	assert.Equal(t, 3, got.OnlineDevices)
	assert.Equal(t, 5, got.PendingTasks)
	assert.Equal(t, domain.NetClassOK, got.NetClass)
}

func TestHubHandleMessageTaskAck(t *testing.T) {
	hub, _, taskMgr, db, cleanup := setupTestHub(t)
	defer cleanup()

	ctx := context.Background()
	userRepo := storage.NewUserRepo(db)
	factoryRepo := storage.NewFactoryRepo(db)
	agentRepo := storage.NewAgentRepo(db)
	deviceRepo := storage.NewDeviceRepo(db)
	require.NoError(t, factoryRepo.Create(ctx, &domain.Factory{FactoryID: "f-1", Name: "F1"}))
	require.NoError(t, agentRepo.Create(ctx, &domain.Agent{AgentID: "a-1", Name: "A1", FactoryID: "f-1", DeviceTokenEnc: []byte("enc")}))
	require.NoError(t, userRepo.Create(ctx, &domain.User{UserID: "u-1", Username: "alice", PasswordHash: "h", PasswordSalt: "", Role: domain.RoleUser, Status: domain.UserStatusActive}))
	require.NoError(t, deviceRepo.Create(ctx, &domain.Device{DeviceID: "d-1", Name: "P", IP: "10.0.0.1", Model: "M", Protocol: domain.ProtocolRAW, Status: domain.DeviceStatusOnline, FactoryID: "f-1", AgentID: "a-1"}))

	task := &domain.PrintTask{TaskID: "t-1", UserID: "u-1", DeviceID: "d-1", AgentID: "a-1", Status: domain.TaskStatusPending, SubmittedAt: time.Now().UTC()}
	require.NoError(t, taskMgr.Submit(ctx, task))

	env := newEnvelope(t, domain.MsgTaskAck, domain.TaskAckPayload{TaskID: "t-1", Accepted: true})
	err := hub.HandleMessage(ctx, "a-1", env)
	require.NoError(t, err)

	got, err := taskMgr.GetTask(ctx, "t-1")
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusRunning, got.Status)
}

func TestHubHandleMessageTaskAckRejected(t *testing.T) {
	hub, _, taskMgr, db, cleanup := setupTestHub(t)
	defer cleanup()

	ctx := context.Background()
	userRepo := storage.NewUserRepo(db)
	factoryRepo := storage.NewFactoryRepo(db)
	agentRepo := storage.NewAgentRepo(db)
	deviceRepo := storage.NewDeviceRepo(db)
	require.NoError(t, factoryRepo.Create(ctx, &domain.Factory{FactoryID: "f-1", Name: "F1"}))
	require.NoError(t, agentRepo.Create(ctx, &domain.Agent{AgentID: "a-1", Name: "A1", FactoryID: "f-1", DeviceTokenEnc: []byte("enc")}))
	require.NoError(t, userRepo.Create(ctx, &domain.User{UserID: "u-1", Username: "alice", PasswordHash: "h", PasswordSalt: "", Role: domain.RoleUser, Status: domain.UserStatusActive}))
	require.NoError(t, deviceRepo.Create(ctx, &domain.Device{DeviceID: "d-1", Name: "P", IP: "10.0.0.1", Model: "M", Protocol: domain.ProtocolRAW, Status: domain.DeviceStatusOnline, FactoryID: "f-1", AgentID: "a-1"}))

	task := &domain.PrintTask{TaskID: "t-2", UserID: "u-1", DeviceID: "d-1", AgentID: "a-1", Status: domain.TaskStatusPending, SubmittedAt: time.Now().UTC()}
	require.NoError(t, taskMgr.Submit(ctx, task))

	env := newEnvelope(t, domain.MsgTaskAck, domain.TaskAckPayload{TaskID: "t-2", Accepted: false, Reason: "device busy"})
	err := hub.HandleMessage(ctx, "a-1", env)
	require.NoError(t, err)

	got, err := taskMgr.GetTask(ctx, "t-2")
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusFailed, got.Status)
	assert.Equal(t, "TASK_REJECTED", got.ErrorCode)
}

func TestHubHandleMessageTaskResult(t *testing.T) {
	hub, _, taskMgr, db, cleanup := setupTestHub(t)
	defer cleanup()

	ctx := context.Background()
	userRepo := storage.NewUserRepo(db)
	factoryRepo := storage.NewFactoryRepo(db)
	agentRepo := storage.NewAgentRepo(db)
	deviceRepo := storage.NewDeviceRepo(db)
	require.NoError(t, factoryRepo.Create(ctx, &domain.Factory{FactoryID: "f-1", Name: "F1"}))
	require.NoError(t, agentRepo.Create(ctx, &domain.Agent{AgentID: "a-1", Name: "A1", FactoryID: "f-1", DeviceTokenEnc: []byte("enc")}))
	require.NoError(t, userRepo.Create(ctx, &domain.User{UserID: "u-1", Username: "alice", PasswordHash: "h", PasswordSalt: "", Role: domain.RoleUser, Status: domain.UserStatusActive}))
	require.NoError(t, deviceRepo.Create(ctx, &domain.Device{DeviceID: "d-1", Name: "P", IP: "10.0.0.1", Model: "M", Protocol: domain.ProtocolRAW, Status: domain.DeviceStatusOnline, FactoryID: "f-1", AgentID: "a-1"}))

	task := &domain.PrintTask{TaskID: "t-3", UserID: "u-1", DeviceID: "d-1", AgentID: "a-1", Status: domain.TaskStatusRunning, SubmittedAt: time.Now().UTC()}
	require.NoError(t, taskMgr.Submit(ctx, task))

	env := newEnvelope(t, domain.MsgTaskResult, domain.TaskResultPayload{
		TaskID:     "t-3",
		DeviceID:   "d-1",
		Status:     domain.TaskStatusSuccess,
		RetryCount: 0,
		FinishedAt: time.Now().UTC(),
	})
	err := hub.HandleMessage(ctx, "a-1", env)
	require.NoError(t, err)

	got, err := taskMgr.GetTask(ctx, "t-3")
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusSuccess, got.Status)
	assert.False(t, got.FinishedAt.IsZero())
}

func TestHubHandleMessageDeviceStatus(t *testing.T) {
	hub, _, _, _, cleanup := setupTestHub(t)
	defer cleanup()

	env := newEnvelope(t, domain.MsgDeviceStatus, domain.DeviceStatusPayload{
		DeviceID: "d-1",
		Status:   domain.DeviceStatusOnline,
		Protocol: domain.ProtocolIPP,
	})
	err := hub.HandleMessage(context.Background(), "a-1", env)
	require.NoError(t, err)
}

func TestHubHandleMessageNetEvent(t *testing.T) {
	hub, _, _, _, cleanup := setupTestHub(t)
	defer cleanup()

	env := newEnvelope(t, domain.MsgNetEvent, domain.NetEventPayload{
		Class:    domain.NetClassCloudGatewayUnreachable,
		Endpoint: "print.oascii.com",
		Detail:   "connection refused",
		TS:       time.Now().UTC(),
	})
	err := hub.HandleMessage(context.Background(), "a-1", env)
	require.NoError(t, err)
}

func TestHubHandleMessageConfigAck(t *testing.T) {
	hub, _, _, _, cleanup := setupTestHub(t)
	defer cleanup()

	env := newEnvelope(t, domain.MsgConfigAck, domain.ConfigAckPayload{Applied: true, Field: "endpoint"})
	err := hub.HandleMessage(context.Background(), "a-1", env)
	require.NoError(t, err)
}

func TestHubHandleMessageLog(t *testing.T) {
	hub, _, _, _, cleanup := setupTestHub(t)
	defer cleanup()

	env := newEnvelope(t, domain.MsgLog, domain.LogPayload{Level: "info", Event: "print_done", Message: "ok"})
	err := hub.HandleMessage(context.Background(), "a-1", env)
	require.NoError(t, err)
}

func TestHubHandleMessageUpgradeResult(t *testing.T) {
	hub, _, _, _, cleanup := setupTestHub(t)
	defer cleanup()

	env := newEnvelope(t, domain.MsgUpgradeResult, domain.UpgradeResultPayload{
		Success: true,
		FromVer: "0.1.0",
		ToVer:   "0.2.0",
	})
	err := hub.HandleMessage(context.Background(), "a-1", env)
	require.NoError(t, err)
}

func TestHubHandleMessageUnknownType(t *testing.T) {
	hub, _, _, _, cleanup := setupTestHub(t)
	defer cleanup()

	env := newEnvelope(t, "unknown_type", map[string]string{"foo": "bar"})
	err := hub.HandleMessage(context.Background(), "a-1", env)
	require.NoError(t, err)
}

func TestHubHandleMessageInvalidPayload(t *testing.T) {
	hub, _, _, _, cleanup := setupTestHub(t)
	defer cleanup()

	env := &domain.Envelope{
		Type:    domain.MsgHeartbeat,
		TS:      time.Now().UTC(),
		Payload: json.RawMessage(`{invalid json}`),
	}
	err := hub.HandleMessage(context.Background(), "a-1", env)
	assert.Error(t, err)
}

func TestHubSendToAgentOffline(t *testing.T) {
	hub, _, _, _, cleanup := setupTestHub(t)
	defer cleanup()

	env := &domain.DownEnvelope{Type: domain.MsgTask, MsgID: "m-1", Payload: json.RawMessage(`{}`)}
	err := hub.SendToAgent("nonexistent-agent", env)
	assert.Error(t, err)
}

func TestHubSendTaskAgentOffline(t *testing.T) {
	hub, _, _, _, cleanup := setupTestHub(t)
	defer cleanup()

	task := &domain.PrintTask{TaskID: "t-1", DeviceID: "d-1"}
	err := hub.SendTask("nonexistent-agent", task)
	assert.Error(t, err)
}

func TestHubSendControlAgentOffline(t *testing.T) {
	hub, _, _, _, cleanup := setupTestHub(t)
	defer cleanup()

	err := hub.SendControl("nonexistent-agent", domain.ControlPayload{Action: "pause"})
	assert.Error(t, err)
}

func TestHubSendToAgentOnline(t *testing.T) {
	hub, agentMgr, _, _, cleanup := setupTestHub(t)
	defer cleanup()

	serverConn, clientConn, connCleanup := newWSSPair(t)
	defer connCleanup()

	ac := agentMgr.Register("agent-online", serverConn)
	defer agentMgr.Unregister("agent-online")

	env, err := domain.NewDownEnvelope(domain.MsgControl, "msg-1", "", domain.ControlPayload{Action: "test_page", DeviceID: "d-1"})
	require.NoError(t, err)

	err = hub.SendToAgent("agent-online", env)
	require.NoError(t, err)

	select {
	case data := <-ac.SendCh():
		assert.NotEmpty(t, data)
		var decoded domain.DownEnvelope
		require.NoError(t, json.Unmarshal(data, &decoded))
		assert.Equal(t, domain.MsgControl, decoded.Type)
		assert.Equal(t, "msg-1", decoded.MsgID)
	case <-time.After(time.Second):
		t.Fatal("等待发送消息超时")
	}

	_ = clientConn
}

func TestHubSendTaskOnline(t *testing.T) {
	hub, agentMgr, _, _, cleanup := setupTestHub(t)
	defer cleanup()

	serverConn, _, connCleanup := newWSSPair(t)
	defer connCleanup()

	ac := agentMgr.Register("agent-task", serverConn)
	defer agentMgr.Unregister("agent-task")

	task := &domain.PrintTask{
		TaskID:      "t-send-1",
		DeviceID:    "d-1",
		DocumentRef: "/api/v1/documents/doc-1/download",
		Checksum:    "abc123",
		Params:      domain.PrintParams{Copies: 1},
		TraceID:     "trace-1",
	}
	err := hub.SendTask("agent-task", task)
	require.NoError(t, err)

	select {
	case data := <-ac.SendCh():
		assert.NotEmpty(t, data)
		var decoded domain.DownEnvelope
		require.NoError(t, json.Unmarshal(data, &decoded))
		assert.Equal(t, domain.MsgTask, decoded.Type)
		var payload domain.TaskPayload
		require.NoError(t, json.Unmarshal(decoded.Payload, &payload))
		assert.Equal(t, "t-send-1", payload.TaskID)
		assert.Equal(t, "d-1", payload.DeviceID)
	case <-time.After(time.Second):
		t.Fatal("等待任务消息超时")
	}
}

func TestHubBroadcast(t *testing.T) {
	hub, agentMgr, _, _, cleanup := setupTestHub(t)
	defer cleanup()

	serverConn1, _, c1 := newWSSPair(t)
	defer c1()
	serverConn2, _, c2 := newWSSPair(t)
	defer c2()

	ac1 := agentMgr.Register("agent-b1", serverConn1)
	defer agentMgr.Unregister("agent-b1")
	ac2 := agentMgr.Register("agent-b2", serverConn2)
	defer agentMgr.Unregister("agent-b2")

	env, err := domain.NewDownEnvelope(domain.MsgUpgrade, "msg-broadcast", "", domain.UpgradePayload{Version: "0.3.0", URL: "https://example.com/v0.3.0"})
	require.NoError(t, err)
	hub.Broadcast(env)

	for i, ac := range []*agentmanager.AgentConnection{ac1, ac2} {
		select {
		case data := <-ac.SendCh():
			assert.NotEmpty(t, data)
		case <-time.After(time.Second):
			t.Fatalf("agent %d 未收到广播", i+1)
		}
	}
}

func TestHubCheckHeartbeats(t *testing.T) {
	hub, agentMgr, _, _, cleanup := setupTestHub(t)
	defer cleanup()

	serverConn, _, connCleanup := newWSSPair(t)
	defer connCleanup()

	agentMgr.Register("agent-hb", serverConn)
	assert.True(t, agentMgr.IsOnline("agent-hb"))

	hub.checkHeartbeats()
	assert.True(t, agentMgr.IsOnline("agent-hb"))

	time.Sleep(300 * time.Millisecond)
	hub.checkHeartbeats()
	assert.False(t, agentMgr.IsOnline("agent-hb"))
}