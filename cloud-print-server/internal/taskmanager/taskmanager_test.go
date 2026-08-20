package taskmanager

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/cloud-print/server/internal/domain"
	"github.com/cloud-print/server/internal/docstore"
	"github.com/cloud-print/server/internal/observability"
	"github.com/cloud-print/server/internal/storage"
)

type mockDispatcher struct {
	sentTasks     []*domain.PrintTask
	sentControls  []domain.ControlPayload
	taskErr       error
	controlErr    error
}

func (m *mockDispatcher) SendTask(agentID string, task *domain.PrintTask) error {
	if m.taskErr != nil {
		return m.taskErr
	}
	m.sentTasks = append(m.sentTasks, task)
	return nil
}

func (m *mockDispatcher) SendControl(agentID string, control domain.ControlPayload) error {
	if m.controlErr != nil {
		return m.controlErr
	}
	m.sentControls = append(m.sentControls, control)
	return nil
}

type mockNotifier struct {
	notifications []notifRecord
}

type notifRecord struct {
	userID   string
	taskID   string
	status   domain.TaskStatus
	payload  interface{}
}

func (m *mockNotifier) NotifyTaskStatus(userID, taskID string, status domain.TaskStatus, payload interface{}) {
	m.notifications = append(m.notifications, notifRecord{userID: userID, taskID: taskID, status: status, payload: payload})
}

func setupTaskMgr(t *testing.T) (*TaskManager, *storage.DB, *mockDispatcher, *mockNotifier, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	migrationsDir := resolveMigrationsDirT(t)

	err := storage.RunMigrations(dbPath, migrationsDir)
	require.NoError(t, err)

	db, err := storage.Open(dbPath)
	require.NoError(t, err)

	logger := zap.NewNop()
	audit := observability.NewAuditLoggerWrapper(logger)

	docStore := docstore.NewStore(filepath.Join(dir, "docs"), logger)
	taskRepo := storage.NewTaskRepo(db)
	dispatcher := &mockDispatcher{}
	notifier := &mockNotifier{}

	tm := NewTaskManager(taskRepo, docStore, dispatcher, logger, audit)
	tm.SetStatusNotifier(notifier)
	deviceRepo := storage.NewDeviceRepo(db)
	tm.SetDeviceRepo(deviceRepo)

	cleanup := func() {
		_ = db.Close()
		_ = os.RemoveAll(dir)
	}
	return tm, db, dispatcher, notifier, cleanup
}

func resolveMigrationsDirT(t *testing.T) string {
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

func seedUserFactoryAgentDevice(t *testing.T, db *storage.DB) {
	t.Helper()
	ctx := context.Background()
	userRepo := storage.NewUserRepo(db)
	factoryRepo := storage.NewFactoryRepo(db)
	agentRepo := storage.NewAgentRepo(db)
	deviceRepo := storage.NewDeviceRepo(db)

	require.NoError(t, factoryRepo.Create(ctx, &domain.Factory{FactoryID: "f-1", Name: "F1"}))
	require.NoError(t, agentRepo.Create(ctx, &domain.Agent{AgentID: "a-1", Name: "A1", FactoryID: "f-1", DeviceTokenEnc: []byte("enc")}))
	require.NoError(t, userRepo.Create(ctx, &domain.User{UserID: "u-1", Username: "alice", PasswordHash: "h", PasswordSalt: "", Role: domain.RoleUser, Status: domain.UserStatusActive}))
	require.NoError(t, deviceRepo.Create(ctx, &domain.Device{DeviceID: "d-1", Name: "P", IP: "10.0.0.1", Model: "M", Protocol: domain.ProtocolRAW, Status: domain.DeviceStatusOnline, FactoryID: "f-1", AgentID: "a-1"}))
}

func TestTaskManagerCreateTask(t *testing.T) {
	tm, db, _, notifier, cleanup := setupTaskMgr(t)
	defer cleanup()
	seedUserFactoryAgentDevice(t, db)

	ctx := context.Background()
	task, err := tm.CreateTask(ctx, "u-1", "d-1", "doc-1", domain.PrintParams{Copies: 2})
	require.NoError(t, err)
	assert.NotEmpty(t, task.TaskID)
	assert.Equal(t, "u-1", task.UserID)
	assert.Equal(t, "d-1", task.DeviceID)
	assert.Equal(t, "doc-1", task.DocID)
	assert.Equal(t, domain.TaskStatusPending, task.Status)
	assert.Equal(t, "a-1", task.AgentID)
	assert.False(t, task.SubmittedAt.IsZero())

	got, err := tm.GetTask(ctx, task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, task.TaskID, got.TaskID)

	assert.Len(t, notifier.notifications, 1)
	assert.Equal(t, domain.TaskStatusPending, notifier.notifications[0].status)
}

func TestTaskManagerCreateTaskInvalidParams(t *testing.T) {
	tm, _, _, _, cleanup := setupTaskMgr(t)
	defer cleanup()

	ctx := context.Background()
	_, err := tm.CreateTask(ctx, "", "d-1", "doc-1", domain.PrintParams{})
	assert.Error(t, err)

	_, err = tm.CreateTask(ctx, "u-1", "", "doc-1", domain.PrintParams{})
	assert.Error(t, err)

	_, err = tm.CreateTask(ctx, "u-1", "d-1", "", domain.PrintParams{})
	assert.Error(t, err)
}

func TestTaskManagerGetTaskNotFound(t *testing.T) {
	tm, _, _, _, cleanup := setupTaskMgr(t)
	defer cleanup()

	_, err := tm.GetTask(context.Background(), "nonexistent")
	assert.Error(t, err)
}

func TestTaskManagerListTasks(t *testing.T) {
	tm, db, _, _, cleanup := setupTaskMgr(t)
	defer cleanup()
	seedUserFactoryAgentDevice(t, db)

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_, err := tm.CreateTask(ctx, "u-1", "d-1", "doc-1", domain.PrintParams{Copies: i + 1})
		require.NoError(t, err)
	}

	list, err := tm.ListTasks(ctx, "u-1", 10, 0)
	require.NoError(t, err)
	assert.Len(t, list, 3)

	list2, err := tm.ListTasks(ctx, "u-1", 2, 0)
	require.NoError(t, err)
	assert.Len(t, list2, 2)

	list3, err := tm.ListTasks(ctx, "u-1", 10, 2)
	require.NoError(t, err)
	assert.Len(t, list3, 1)

	all, err := tm.ListAll(ctx, 10, 0)
	require.NoError(t, err)
	assert.Len(t, all, 3)
}

func TestTaskManagerCancelTask(t *testing.T) {
	tm, db, dispatcher, _, cleanup := setupTaskMgr(t)
	defer cleanup()
	seedUserFactoryAgentDevice(t, db)

	ctx := context.Background()
	task, err := tm.CreateTask(ctx, "u-1", "d-1", "doc-1", domain.PrintParams{})
	require.NoError(t, err)

	err = tm.CancelTask(ctx, task.TaskID, "u-1")
	require.NoError(t, err)

	got, err := tm.GetTask(ctx, task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusCancelled, got.Status)

	assert.Len(t, dispatcher.sentControls, 1)
	assert.Equal(t, "cancel_task", dispatcher.sentControls[0].Action)
	assert.Equal(t, task.TaskID, dispatcher.sentControls[0].TaskID)
}

func TestTaskManagerCancelTaskWrongUser(t *testing.T) {
	tm, db, _, _, cleanup := setupTaskMgr(t)
	defer cleanup()
	seedUserFactoryAgentDevice(t, db)

	ctx := context.Background()
	task, err := tm.CreateTask(ctx, "u-1", "d-1", "doc-1", domain.PrintParams{})
	require.NoError(t, err)

	err = tm.CancelTask(ctx, task.TaskID, "u-other")
	assert.Error(t, err)
}

func TestTaskManagerCancelTaskNotFound(t *testing.T) {
	tm, _, _, _, cleanup := setupTaskMgr(t)
	defer cleanup()

	err := tm.CancelTask(context.Background(), "nonexistent", "u-1")
	assert.Error(t, err)
}

func TestTaskManagerDispatchTask(t *testing.T) {
	tm, db, dispatcher, _, cleanup := setupTaskMgr(t)
	defer cleanup()
	seedUserFactoryAgentDevice(t, db)

	ctx := context.Background()
	task, err := tm.CreateTask(ctx, "u-1", "d-1", "doc-1", domain.PrintParams{Copies: 1})
	require.NoError(t, err)

	err = tm.DispatchTask(ctx, task.TaskID)
	require.NoError(t, err)

	assert.Len(t, dispatcher.sentTasks, 1)
	assert.Equal(t, task.TaskID, dispatcher.sentTasks[0].TaskID)

	got, err := tm.GetTask(ctx, task.TaskID)
	require.NoError(t, err)
	assert.False(t, got.DispatchedAt.IsZero())
}

func TestTaskManagerDispatchTaskAlreadyDispatched(t *testing.T) {
	tm, db, _, _, cleanup := setupTaskMgr(t)
	defer cleanup()
	seedUserFactoryAgentDevice(t, db)

	ctx := context.Background()
	task, err := tm.CreateTask(ctx, "u-1", "d-1", "doc-1", domain.PrintParams{})
	require.NoError(t, err)

	require.NoError(t, tm.DispatchTask(ctx, task.TaskID))
	err = tm.DispatchTask(ctx, task.TaskID)
	assert.Error(t, err)
}

func TestTaskManagerHandleAckAccepted(t *testing.T) {
	tm, db, _, notifier, cleanup := setupTaskMgr(t)
	defer cleanup()
	seedUserFactoryAgentDevice(t, db)

	ctx := context.Background()
	task, err := tm.CreateTask(ctx, "u-1", "d-1", "doc-1", domain.PrintParams{})
	require.NoError(t, err)
	require.NoError(t, tm.DispatchTask(ctx, task.TaskID))

	notifier.notifications = nil
	err = tm.HandleAck(ctx, domain.TaskAckPayload{TaskID: task.TaskID, Accepted: true})
	require.NoError(t, err)

	got, err := tm.GetTask(ctx, task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusRunning, got.Status)
	assert.False(t, got.StartedAt.IsZero())

	assert.Len(t, notifier.notifications, 1)
	assert.Equal(t, domain.TaskStatusRunning, notifier.notifications[0].status)
}

func TestTaskManagerHandleAckRejected(t *testing.T) {
	tm, db, _, _, cleanup := setupTaskMgr(t)
	defer cleanup()
	seedUserFactoryAgentDevice(t, db)

	ctx := context.Background()
	task, err := tm.CreateTask(ctx, "u-1", "d-1", "doc-1", domain.PrintParams{})
	require.NoError(t, err)

	err = tm.HandleAck(ctx, domain.TaskAckPayload{TaskID: task.TaskID, Accepted: false, Reason: "device offline"})
	require.NoError(t, err)

	got, err := tm.GetTask(ctx, task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusFailed, got.Status)
	assert.Equal(t, "TASK_REJECTED", got.ErrorCode)
	assert.Equal(t, "device offline", got.ErrorMsg)
}

func TestTaskManagerHandleResultSuccess(t *testing.T) {
	tm, db, _, _, cleanup := setupTaskMgr(t)
	defer cleanup()
	seedUserFactoryAgentDevice(t, db)

	ctx := context.Background()
	task, err := tm.CreateTask(ctx, "u-1", "d-1", "doc-1", domain.PrintParams{})
	require.NoError(t, err)
	require.NoError(t, tm.HandleAck(ctx, domain.TaskAckPayload{TaskID: task.TaskID, Accepted: true}))

	err = tm.HandleResult(ctx, domain.TaskResultPayload{
		TaskID:     task.TaskID,
		DeviceID:   "d-1",
		Status:     domain.TaskStatusSuccess,
		RetryCount: 0,
		FinishedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	got, err := tm.GetTask(ctx, task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusSuccess, got.Status)
	assert.False(t, got.FinishedAt.IsZero())
}

func TestTaskManagerHandleResultFailed(t *testing.T) {
	tm, db, _, _, cleanup := setupTaskMgr(t)
	defer cleanup()
	seedUserFactoryAgentDevice(t, db)

	ctx := context.Background()
	task, err := tm.CreateTask(ctx, "u-1", "d-1", "doc-1", domain.PrintParams{})
	require.NoError(t, err)
	require.NoError(t, tm.HandleAck(ctx, domain.TaskAckPayload{TaskID: task.TaskID, Accepted: true}))

	err = tm.HandleResult(ctx, domain.TaskResultPayload{
		TaskID:     task.TaskID,
		DeviceID:   "d-1",
		Status:     domain.TaskStatusFailed,
		RetryCount: 1,
		ErrorCode:  "PAPER_JAM",
		ErrorMsg:   "纸张卡住",
		FinishedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	got, err := tm.GetTask(ctx, task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusFailed, got.Status)
	assert.Equal(t, 1, got.RetryCount)
	assert.Equal(t, "PAPER_JAM", got.ErrorCode)
}

func TestTaskManagerTransitionPendingToRunning(t *testing.T) {
	tm, db, _, _, cleanup := setupTaskMgr(t)
	defer cleanup()
	seedUserFactoryAgentDevice(t, db)

	ctx := context.Background()
	task, err := tm.CreateTask(ctx, "u-1", "d-1", "doc-1", domain.PrintParams{})
	require.NoError(t, err)

	err = tm.Transition(ctx, task.TaskID, domain.TaskStatusRunning)
	require.NoError(t, err)

	got, err := tm.GetTask(ctx, task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusRunning, got.Status)
}

func TestTaskManagerTransitionTerminalToRunning(t *testing.T) {
	tm, db, _, _, cleanup := setupTaskMgr(t)
	defer cleanup()
	seedUserFactoryAgentDevice(t, db)

	ctx := context.Background()
	task, err := tm.CreateTask(ctx, "u-1", "d-1", "doc-1", domain.PrintParams{})
	require.NoError(t, err)
	require.NoError(t, tm.Transition(ctx, task.TaskID, domain.TaskStatusRunning))
	require.NoError(t, tm.Transition(ctx, task.TaskID, domain.TaskStatusSuccess))

	err = tm.Transition(ctx, task.TaskID, domain.TaskStatusRunning)
	require.NoError(t, err)

	got, err := tm.GetTask(ctx, task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusSuccess, got.Status)
}

func TestTaskManagerStatusChange(t *testing.T) {
	tm, db, _, _, cleanup := setupTaskMgr(t)
	defer cleanup()
	seedUserFactoryAgentDevice(t, db)

	ctx := context.Background()
	task, err := tm.CreateTask(ctx, "u-1", "d-1", "doc-1", domain.PrintParams{})
	require.NoError(t, err)

	change := StatusChange{
		TaskID:    task.TaskID,
		UserID:    "u-1",
		Status:    domain.TaskStatusRunning,
		Previous:  domain.TaskStatusPending,
		ChangedAt: time.Now().UTC(),
	}
	assert.Equal(t, task.TaskID, change.TaskID)
	assert.Equal(t, domain.TaskStatusRunning, change.Status)
}