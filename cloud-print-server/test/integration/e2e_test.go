package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"nhooyr.io/websocket"

	"github.com/cloud-print/server/internal/agentmanager"
	"github.com/cloud-print/server/internal/auth"
	"github.com/cloud-print/server/internal/devicemanager"
	"github.com/cloud-print/server/internal/docstore"
	"github.com/cloud-print/server/internal/domain"
	"github.com/cloud-print/server/internal/observability"
	"github.com/cloud-print/server/internal/restapi"
	"github.com/cloud-print/server/internal/storage"
	"github.com/cloud-print/server/internal/taskmanager"
	"github.com/cloud-print/server/internal/wsshub"
)

type e2eFixture struct {
	server      *httptest.Server
	db          *storage.DB
	agentMgr    *agentmanager.Manager
	taskMgr     *taskmanager.TaskManager
	hub         *wsshub.Hub
	jwtMgr      *auth.JWTManager
	credMgr     *agentmanager.CredentialManager
	agentToken  string
	agentID     string
	userID      string
	deviceID    string
	factoryID   string
	userToken   string
	docID       string
}

func setupE2E(t *testing.T) (*e2eFixture, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	migrationsDir := resolveMigrationsDirE2E(t)

	require.NoError(t, storage.RunMigrations(dbPath, migrationsDir))
	db, err := storage.Open(dbPath)
	require.NoError(t, err)

	logger := zap.NewNop()
	audit := observability.NewAuditLoggerWrapper(logger)

	userRepo := storage.NewUserRepo(db)
	factoryRepo := storage.NewFactoryRepo(db)
	agentRepo := storage.NewAgentRepo(db)
	deviceRepo := storage.NewDeviceRepo(db)
	taskRepo := storage.NewTaskRepo(db)
	docRepo := storage.NewDocumentRepo(db)

	jwtMgr := auth.NewJWTManager("e2e-jwt-secret", 12)
	credMgr := agentmanager.NewCredentialManager("e2e-master-key")

	agentMgrRepo := agentmanager.NewRepo(agentRepo)
	agentMgr := agentmanager.NewManager(agentMgrRepo, credMgr, logger)

	deviceMgr := devicemanager.NewManager(deviceRepo, logger)
	deviceMgr.SetUserRepo(userRepo)

	docStore := docstore.NewStore(filepath.Join(dir, "docs"), logger)
	taskMgr := taskmanager.NewTaskManager(taskRepo, docStore, nil, logger, audit)
	taskMgr.SetDeviceRepo(deviceRepo)

	hub := wsshub.NewHub(agentMgr, taskMgr, logger, audit)
	taskMgr.SetHub(hub)

	sseHub := restapi.NewSSEHub(logger)
	taskMgr.SetStatusNotifier(sseHub)

	router := chi.NewRouter()
	restHandlers := restapi.NewHandlers(restapi.HandlersConfig{
		JWTMgr:            jwtMgr,
		UserRepo:          userRepo,
		DocRepo:           docRepo,
		TaskMgr:           taskMgr,
		DeviceMgr:         deviceMgr,
		DocStore:          docStore,
		SSEHub:            sseHub,
		Logger:            logger,
		Audit:             audit,
		BcryptCost:        10,
		MaxDocSizeMB:      50,
		DocRetentionHours: 24,
	})
	restapi.RegisterRoutes(router, restHandlers)
	router.Get("/agent", hub.HandleAgentWS)
	router.Get("/api/v1/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	srv := httptest.NewServer(router)

	fix := &e2eFixture{
		server:   srv,
		db:       db,
		agentMgr: agentMgr,
		taskMgr:  taskMgr,
		hub:      hub,
		jwtMgr:   jwtMgr,
		credMgr:  credMgr,
	}

	ctx := context.Background()
	fix.factoryID = "f-e2e"
	require.NoError(t, factoryRepo.Create(ctx, &domain.Factory{FactoryID: fix.factoryID, Name: "E2E工厂", Code: "E2E"}))

	fix.agentID = "a-e2e"
	token, enc, err := credMgr.Generate(fix.agentID)
	require.NoError(t, err)
	fix.agentToken = token
	agent := &domain.Agent{
		AgentID:        fix.agentID,
		Name:           "E2E Agent",
		FactoryID:      fix.factoryID,
		DeviceTokenEnc: enc,
		Version:        "0.1.0",
	}
	require.NoError(t, agentRepo.Create(ctx, agent))

	fix.userID = "u-e2e"
	pwHash, _, err := auth.HashPassword("Admin@123", 10)
	require.NoError(t, err)
	require.NoError(t, userRepo.Create(ctx, &domain.User{
		UserID:       fix.userID,
		Username:     "admin",
		PasswordHash: pwHash,
		Role:         domain.RoleAdmin,
		Status:       domain.UserStatusActive,
		DisplayName:  "管理员",
	}))

	fix.deviceID = "d-e2e"
	require.NoError(t, deviceRepo.Create(ctx, &domain.Device{
		DeviceID:  fix.deviceID,
		Name:      "E2E打印机",
		IP:        "127.0.0.1",
		Model:     "TestPrinter",
		Protocol:  domain.ProtocolRAW,
		Status:    domain.DeviceStatusOnline,
		FactoryID: fix.factoryID,
		AgentID:   fix.agentID,
		Port:      9100,
	}))
	require.NoError(t, userRepo.GrantPermission(ctx, fix.userID, fix.deviceID))

	cleanup := func() {
		srv.Close()
		_ = db.Close()
		_ = os.RemoveAll(dir)
	}
	return fix, cleanup
}

func resolveMigrationsDirE2E(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"../../migrations",
		"../../../migrations",
		"../../../../migrations",
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

func (f *e2eFixture) wsURL() string {
	return "ws" + strings.TrimPrefix(f.server.URL, "http") + "/agent?agent_id=" + f.agentID + "&token=" + f.agentToken
}

func (f *e2eFixture) login(t *testing.T) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "Admin@123"})
	resp, err := http.Post(f.server.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		Code string `json:"code"`
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	require.Equal(t, "OK", result.Code)
	require.NotEmpty(t, result.Data.Token)
	return result.Data.Token
}

func (f *e2eFixture) uploadDocument(t *testing.T, token, filename, content string) string {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fileWriter, err := mw.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = io.WriteString(fileWriter, content)
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	req, err := http.NewRequest("POST", f.server.URL+"/api/v1/documents/upload", &buf)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var result struct {
		Data struct {
			DocID string `json:"doc_id"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	require.NotEmpty(t, result.Data.DocID)
	return result.Data.DocID
}

func (f *e2eFixture) createTask(t *testing.T, token, deviceID, docID string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{
		"device_id":   deviceID,
		"doc_id":      docID,
		"copies":      1,
		"orientation": "portrait",
	})
	req, err := http.NewRequest("POST", f.server.URL+"/api/v1/tasks/", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var result struct {
		Data struct {
			TaskID string `json:"task_id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	require.NotEmpty(t, result.Data.TaskID)
	return result.Data.TaskID
}

func (f *e2eFixture) getTask(t *testing.T, token, taskID string) (string, string) {
	t.Helper()
	req, err := http.NewRequest("GET", f.server.URL+"/api/v1/tasks/"+taskID+"/", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		Data struct {
			TaskID    string `json:"task_id"`
			Status    string `json:"status"`
			ErrorCode string `json:"error_code"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	return result.Data.Status, result.Data.ErrorCode
}

func dialAgent(t *testing.T, wsURL string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{},
	})
	require.NoError(t, err)
	return conn
}

func readEnvelope(t *testing.T, conn *websocket.Conn) domain.DownEnvelope {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, data, err := conn.Read(ctx)
	require.NoError(t, err)
	var env domain.DownEnvelope
	require.NoError(t, json.Unmarshal(data, &env))
	return env
}

func sendEnvelope(t *testing.T, conn *websocket.Conn, msgType string, payload interface{}) {
	t.Helper()
	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)
	env := domain.Envelope{
		Type:    msgType,
		TS:      time.Now().UTC(),
		Payload: payloadBytes,
	}
	data, err := json.Marshal(env)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, conn.Write(ctx, websocket.MessageText, data))
}

func TestE2ELoginAndAuth(t *testing.T) {
	fix, cleanup := setupE2E(t)
	defer cleanup()

	token := fix.login(t)
	assert.NotEmpty(t, token)
}

func TestE2ELoginInvalidCredentials(t *testing.T) {
	fix, cleanup := setupE2E(t)
	defer cleanup()

	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "wrong"})
	resp, err := http.Post(fix.server.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestE2EUploadAndTaskFlow(t *testing.T) {
	fix, cleanup := setupE2E(t)
	defer cleanup()

	token := fix.login(t)
	docID := fix.uploadDocument(t, token, "test.txt", "hello print job")
	taskID := fix.createTask(t, token, fix.deviceID, docID)

	time.Sleep(500 * time.Millisecond)

	status, _ := fix.getTask(t, token, taskID)
	assert.True(t, status == "PENDING" || status == "RUNNING" || status == "DISPATCHED", "unexpected status: %s", status)
}

func TestE2EFullTaskLifecycleWithMockAgent(t *testing.T) {
	fix, cleanup := setupE2E(t)
	defer cleanup()

	agentConn := dialAgent(t, fix.wsURL())
	defer agentConn.Close(websocket.StatusNormalClosure, "")

	time.Sleep(200 * time.Millisecond)
	assert.True(t, fix.agentMgr.IsOnline(fix.agentID), "mock agent 应在线")

	token := fix.login(t)
	docID := fix.uploadDocument(t, token, "report.pdf", "%PDF-1.4 mock content for e2e test")
	taskID := fix.createTask(t, token, fix.deviceID, docID)

	env := readEnvelope(t, agentConn)
	assert.Equal(t, domain.MsgTask, env.Type)

	var taskPayload domain.TaskPayload
	require.NoError(t, json.Unmarshal(env.Payload, &taskPayload))
	assert.Equal(t, taskID, taskPayload.TaskID)
	assert.Equal(t, fix.deviceID, taskPayload.DeviceID)

	sendEnvelope(t, agentConn, domain.MsgTaskAck, domain.TaskAckPayload{
		TaskID:   taskID,
		Accepted: true,
	})

	time.Sleep(200 * time.Millisecond)
	status, _ := fix.getTask(t, token, taskID)
	assert.Equal(t, "RUNNING", status)

	sendEnvelope(t, agentConn, domain.MsgTaskResult, domain.TaskResultPayload{
		TaskID:     taskID,
		DeviceID:   fix.deviceID,
		Status:     domain.TaskStatusSuccess,
		RetryCount: 0,
		FinishedAt: time.Now().UTC(),
	})

	time.Sleep(200 * time.Millisecond)
	status, _ = fix.getTask(t, token, taskID)
	assert.Equal(t, "SUCCESS", status)
}

func TestE2ETaskRejectedByAgent(t *testing.T) {
	fix, cleanup := setupE2E(t)
	defer cleanup()

	agentConn := dialAgent(t, fix.wsURL())
	defer agentConn.Close(websocket.StatusNormalClosure, "")

	time.Sleep(200 * time.Millisecond)

	token := fix.login(t)
	docID := fix.uploadDocument(t, token, "doc.txt", "content")
	taskID := fix.createTask(t, token, fix.deviceID, docID)

	env := readEnvelope(t, agentConn)
	assert.Equal(t, domain.MsgTask, env.Type)

	sendEnvelope(t, agentConn, domain.MsgTaskAck, domain.TaskAckPayload{
		TaskID:   taskID,
		Accepted: false,
		Reason:   "device offline",
	})

	time.Sleep(200 * time.Millisecond)
	status, errCode := fix.getTask(t, token, taskID)
	assert.Equal(t, "FAILED", status)
	assert.Equal(t, "TASK_REJECTED", errCode)
}

func TestE2ETaskFailedWithRetry(t *testing.T) {
	fix, cleanup := setupE2E(t)
	defer cleanup()

	agentConn := dialAgent(t, fix.wsURL())
	defer agentConn.Close(websocket.StatusNormalClosure, "")

	time.Sleep(200 * time.Millisecond)

	token := fix.login(t)
	docID := fix.uploadDocument(t, token, "doc.txt", "content")
	taskID := fix.createTask(t, token, fix.deviceID, docID)

	env := readEnvelope(t, agentConn)
	assert.Equal(t, domain.MsgTask, env.Type)

	sendEnvelope(t, agentConn, domain.MsgTaskAck, domain.TaskAckPayload{
		TaskID:   taskID,
		Accepted: true,
	})

	time.Sleep(200 * time.Millisecond)

	sendEnvelope(t, agentConn, domain.MsgTaskResult, domain.TaskResultPayload{
		TaskID:     taskID,
		DeviceID:   fix.deviceID,
		Status:     domain.TaskStatusFailed,
		RetryCount: 1,
		ErrorCode:  "PAPER_JAM",
		ErrorMsg:   "纸张卡住",
		FinishedAt: time.Now().UTC(),
	})

	time.Sleep(200 * time.Millisecond)
	status, errCode := fix.getTask(t, token, taskID)
	assert.Equal(t, "FAILED", status)
	assert.Equal(t, "PAPER_JAM", errCode)
}

func TestE2EAgentHeartbeat(t *testing.T) {
	fix, cleanup := setupE2E(t)
	defer cleanup()

	agentConn := dialAgent(t, fix.wsURL())
	defer agentConn.Close(websocket.StatusNormalClosure, "")

	time.Sleep(200 * time.Millisecond)

	sendEnvelope(t, agentConn, domain.MsgHeartbeat, domain.HeartbeatPayload{
		AgentID:       fix.agentID,
		Version:       "0.2.0",
		OnlineDevices: 1,
		PendingTasks:  0,
		CloudEndpoint: "print.oascii.com",
		NetClass:      domain.NetClassOK,
		Timestamp:     time.Now().UTC(),
	})

	time.Sleep(200 * time.Millisecond)

	ac, ok := fix.agentMgr.Get(fix.agentID)
	require.True(t, ok)
	assert.Equal(t, "0.2.0", ac.Version)
	assert.Equal(t, 1, ac.OnlineDevices)
}

func TestE2EHealthz(t *testing.T) {
	fix, cleanup := setupE2E(t)
	defer cleanup()

	resp, err := http.Get(fix.server.URL + "/api/v1/healthz")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "ok")
}

func TestE2EAgentAuthInvalidToken(t *testing.T) {
	fix, cleanup := setupE2E(t)
	defer cleanup()

	badURL := "ws" + strings.TrimPrefix(fix.server.URL, "http") + "/agent?agent_id=" + fix.agentID + "&token=invalid-token"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, badURL, &websocket.DialOptions{HTTPHeader: http.Header{}})
	if err == nil {
		_, _, readErr := conn.Read(ctx)
		assert.Error(t, readErr)
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}
}

func TestE2EMultipleAgentsAndBroadcast(t *testing.T) {
	fix, cleanup := setupE2E(t)
	defer cleanup()

	agentRepo := storage.NewAgentRepo(fix.db)
	credMgr := fix.credMgr

	agent2ID := "a-e2e-2"
	token2, enc2, err := credMgr.Generate(agent2ID)
	require.NoError(t, err)
	require.NoError(t, agentRepo.Create(context.Background(), &domain.Agent{
		AgentID:        agent2ID,
		Name:           "E2E Agent 2",
		FactoryID:      fix.factoryID,
		DeviceTokenEnc: enc2,
		Version:        "0.1.0",
	}))

	conn1 := dialAgent(t, fix.wsURL())
	defer conn1.Close(websocket.StatusNormalClosure, "")

	wsURL2 := "ws" + strings.TrimPrefix(fix.server.URL, "http") + "/agent?agent_id=" + agent2ID + "&token=" + token2
	conn2 := dialAgent(t, wsURL2)
	defer conn2.Close(websocket.StatusNormalClosure, "")

	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, 2, fix.agentMgr.OnlineCount())

	env, err := domain.NewDownEnvelope(domain.MsgUpgrade, "msg-upgrade", "", domain.UpgradePayload{
		Version: "0.3.0",
		URL:     "https://example.com/v0.3.0",
	})
	require.NoError(t, err)
	fix.hub.Broadcast(env)

	readCtx1, cancel1 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel1()
	_, data1, err := conn1.Read(readCtx1)
	require.NoError(t, err)
	assert.Contains(t, string(data1), "upgrade")

	readCtx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	_, data2, err := conn2.Read(readCtx2)
	require.NoError(t, err)
	assert.Contains(t, string(data2), "upgrade")
}

func TestE2ECancelTask(t *testing.T) {
	fix, cleanup := setupE2E(t)
	defer cleanup()

	agentConn := dialAgent(t, fix.wsURL())
	defer agentConn.Close(websocket.StatusNormalClosure, "")

	time.Sleep(200 * time.Millisecond)

	token := fix.login(t)
	docID := fix.uploadDocument(t, token, "doc.txt", "content")
	taskID := fix.createTask(t, token, fix.deviceID, docID)

	env := readEnvelope(t, agentConn)
	assert.Equal(t, domain.MsgTask, env.Type)

	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/tasks/%s/", fix.server.URL, taskID), nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	time.Sleep(200 * time.Millisecond)
	status, _ := fix.getTask(t, token, taskID)
	assert.Equal(t, "CANCELLED", status)
}