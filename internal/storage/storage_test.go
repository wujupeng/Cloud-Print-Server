package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloud-print/server/internal/domain"
)

func setupTestDB(t *testing.T) (*DB, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	migrationsDir := resolveMigrationsDir(t)

	err := RunMigrations(dbPath, migrationsDir)
	require.NoError(t, err)

	db, err := Open(dbPath)
	require.NoError(t, err)

	cleanup := func() {
		_ = db.Close()
		_ = os.RemoveAll(dir)
	}
	return db, cleanup
}

func resolveMigrationsDir(t *testing.T) string {
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

func TestRunMigrations(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "m.db")
	migrationsDir := resolveMigrationsDir(t)

	err := RunMigrations(dbPath, migrationsDir)
	require.NoError(t, err)

	db, err := Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	var name string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='users'").Scan(&name)
	require.NoError(t, err)
	assert.Equal(t, "users", name)

	err = RunMigrations(dbPath, migrationsDir)
	assert.NoError(t, err)
}

func TestUserRepoCRUD(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewUserRepo(db)
	ctx := context.Background()

	u := &domain.User{
		UserID:      "u-1",
		Username:    "alice",
		PasswordHash: "$2a$10$dummyhash",
		PasswordSalt: "",
		Role:        domain.RoleUser,
		Status:      domain.UserStatusActive,
		DisplayName: "Alice",
	}
	require.NoError(t, repo.Create(ctx, u))

	got, err := repo.GetByID(ctx, "u-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "alice", got.Username)
	assert.Equal(t, domain.RoleUser, got.Role)

	gotByName, err := repo.GetByUsername(ctx, "alice")
	require.NoError(t, err)
	require.NotNil(t, gotByName)
	assert.Equal(t, "u-1", gotByName.UserID)

	u.DisplayName = "Alice Updated"
	u.Role = domain.RoleAdmin
	require.NoError(t, repo.Update(ctx, u))

	updated, err := repo.GetByID(ctx, "u-1")
	require.NoError(t, err)
	assert.Equal(t, "Alice Updated", updated.DisplayName)
	assert.Equal(t, domain.RoleAdmin, updated.Role)

	list, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	require.NoError(t, repo.Delete(ctx, "u-1"))
	deleted, err := repo.GetByID(ctx, "u-1")
	require.NoError(t, err)
	assert.Nil(t, deleted)
}

func TestUserRepoPermissions(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	userRepo := NewUserRepo(db)
	deviceRepo := NewDeviceRepo(db)
	factoryRepo := NewFactoryRepo(db)
	agentRepo := NewAgentRepo(db)
	ctx := context.Background()

	require.NoError(t, factoryRepo.Create(ctx, &domain.Factory{FactoryID: "f-1", Name: "F1"}))
	require.NoError(t, agentRepo.Create(ctx, &domain.Agent{AgentID: "a-1", Name: "A1", FactoryID: "f-1", DeviceTokenEnc: []byte("enc")}))
	require.NoError(t, userRepo.Create(ctx, &domain.User{UserID: "u-1", Username: "bob", PasswordHash: "h", PasswordSalt: "", Role: domain.RoleUser, Status: domain.UserStatusActive}))
	require.NoError(t, deviceRepo.Create(ctx, &domain.Device{DeviceID: "d-1", Name: "Printer", IP: "10.0.0.1", Model: "M", Protocol: domain.ProtocolRAW, Status: domain.DeviceStatusOnline, FactoryID: "f-1", AgentID: "a-1"}))

	require.NoError(t, userRepo.GrantPermission(ctx, "u-1", "d-1"))
	has, err := userRepo.HasPermission(ctx, "u-1", "d-1")
	require.NoError(t, err)
	assert.True(t, has)

	perms, err := userRepo.ListPermissions(ctx, "u-1")
	require.NoError(t, err)
	assert.Contains(t, perms, "d-1")

	require.NoError(t, userRepo.RevokePermission(ctx, "u-1", "d-1"))
	has, err = userRepo.HasPermission(ctx, "u-1", "d-1")
	require.NoError(t, err)
	assert.False(t, has)
}

func TestFactoryRepoCRUD(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewFactoryRepo(db)
	ctx := context.Background()

	f := &domain.Factory{FactoryID: "f-1", Name: "工厂A", Code: "FA", Location: "上海"}
	require.NoError(t, repo.Create(ctx, f))

	got, err := repo.GetByID(ctx, "f-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "工厂A", got.Name)

	f.Name = "工厂A-改"
	require.NoError(t, repo.Update(ctx, f))
	updated, err := repo.GetByID(ctx, "f-1")
	require.NoError(t, err)
	assert.Equal(t, "工厂A-改", updated.Name)

	list, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	require.NoError(t, repo.Delete(ctx, "f-1"))
	deleted, err := repo.GetByID(ctx, "f-1")
	require.NoError(t, err)
	assert.Nil(t, deleted)
}

func TestAgentRepoCRUD(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	factoryRepo := NewFactoryRepo(db)
	repo := NewAgentRepo(db)
	ctx := context.Background()

	require.NoError(t, factoryRepo.Create(ctx, &domain.Factory{FactoryID: "f-1", Name: "F1"}))

	a := &domain.Agent{
		AgentID:        "a-1",
		Name:           "宝山Agent",
		FactoryID:      "f-1",
		DeviceTokenEnc: []byte("encrypted-token"),
		Version:        "0.1.0",
		Online:         false,
	}
	require.NoError(t, repo.Create(ctx, a))

	got, err := repo.GetByID(ctx, "a-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "宝山Agent", got.Name)
	assert.Equal(t, "encrypted-token", string(got.DeviceTokenEnc))

	require.NoError(t, repo.SetOnline(ctx, "a-1", true))
	onlined, err := repo.GetByID(ctx, "a-1")
	require.NoError(t, err)
	assert.True(t, onlined.Online)

	require.NoError(t, repo.UpdateHeartbeat(ctx, "a-1", "0.2.0", 3, 5, domain.NetClassOK))
	hb, err := repo.GetByID(ctx, "a-1")
	require.NoError(t, err)
	assert.Equal(t, "0.2.0", hb.Version)
	assert.Equal(t, 3, hb.OnlineDevices)
	assert.Equal(t, 5, hb.PendingTasks)

	require.NoError(t, repo.UpdateVersion(ctx, "a-1", "0.3.0"))
	ver, err := repo.GetByID(ctx, "a-1")
	require.NoError(t, err)
	assert.Equal(t, "0.3.0", ver.Version)

	require.NoError(t, repo.UpdateNetClass(ctx, "a-1", domain.NetClassLocalNetFail))
	nc, err := repo.GetByID(ctx, "a-1")
	require.NoError(t, err)
	assert.Equal(t, domain.NetClassLocalNetFail, nc.NetClass)

	list, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	require.NoError(t, repo.Delete(ctx, "a-1"))
	deleted, err := repo.GetByID(ctx, "a-1")
	require.NoError(t, err)
	assert.Nil(t, deleted)
}

func TestDeviceRepoCRUD(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	factoryRepo := NewFactoryRepo(db)
	agentRepo := NewAgentRepo(db)
	repo := NewDeviceRepo(db)
	ctx := context.Background()

	require.NoError(t, factoryRepo.Create(ctx, &domain.Factory{FactoryID: "f-1", Name: "F1"}))
	require.NoError(t, agentRepo.Create(ctx, &domain.Agent{AgentID: "a-1", Name: "A1", FactoryID: "f-1", DeviceTokenEnc: []byte("enc")}))

	d := &domain.Device{
		DeviceID:  "d-1",
		Name:      "打印机1",
		IP:        "192.168.1.100",
		Hostname:  "printer1.local",
		Model:     "HP-LaserJet",
		Protocol:  domain.ProtocolIPP,
		Status:    domain.DeviceStatusOnline,
		FactoryID: "f-1",
		AgentID:   "a-1",
		Port:      631,
	}
	require.NoError(t, repo.Create(ctx, d))

	got, err := repo.GetByID(ctx, "d-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "打印机1", got.Name)
	assert.Equal(t, domain.ProtocolIPP, got.Protocol)
	assert.Equal(t, 631, got.Port)

	require.NoError(t, repo.UpdateStatus(ctx, "d-1", domain.DeviceStatusOffline, domain.ProtocolRAW))
	updated, err := repo.GetByID(ctx, "d-1")
	require.NoError(t, err)
	assert.Equal(t, domain.DeviceStatusOffline, updated.Status)
	assert.Equal(t, domain.ProtocolRAW, updated.Protocol)

	d.Name = "打印机1-改"
	require.NoError(t, repo.Update(ctx, d))
	updated, err = repo.GetByID(ctx, "d-1")
	require.NoError(t, err)
	assert.Equal(t, "打印机1-改", updated.Name)

	require.NoError(t, repo.Create(ctx, &domain.Device{DeviceID: "d-2", Name: "P2", IP: "10.0.0.2", Model: "M", Protocol: domain.ProtocolRAW, Status: domain.DeviceStatusOnline, FactoryID: "f-1", AgentID: "a-1"}))

	byFactory, err := repo.ListByFactory(ctx, "f-1")
	require.NoError(t, err)
	assert.Len(t, byFactory, 2)

	byAgent, err := repo.ListByAgent(ctx, "a-1")
	require.NoError(t, err)
	assert.Len(t, byAgent, 2)

	all, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	require.NoError(t, repo.Delete(ctx, "d-1"))
	deleted, err := repo.GetByID(ctx, "d-1")
	require.NoError(t, err)
	assert.Nil(t, deleted)
}

func TestTaskRepoCRUD(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	factoryRepo := NewFactoryRepo(db)
	agentRepo := NewAgentRepo(db)
	deviceRepo := NewDeviceRepo(db)
	userRepo := NewUserRepo(db)
	repo := NewTaskRepo(db)
	ctx := context.Background()

	require.NoError(t, factoryRepo.Create(ctx, &domain.Factory{FactoryID: "f-1", Name: "F1"}))
	require.NoError(t, agentRepo.Create(ctx, &domain.Agent{AgentID: "a-1", Name: "A1", FactoryID: "f-1", DeviceTokenEnc: []byte("enc")}))
	require.NoError(t, userRepo.Create(ctx, &domain.User{UserID: "u-1", Username: "alice", PasswordHash: "h", PasswordSalt: "", Role: domain.RoleUser, Status: domain.UserStatusActive}))
	require.NoError(t, deviceRepo.Create(ctx, &domain.Device{DeviceID: "d-1", Name: "P", IP: "10.0.0.1", Model: "M", Protocol: domain.ProtocolRAW, Status: domain.DeviceStatusOnline, FactoryID: "f-1", AgentID: "a-1"}))

	now := time.Now().UTC()
	task := &domain.PrintTask{
		TaskID:      "t-1",
		UserID:      "u-1",
		DeviceID:    "d-1",
		AgentID:     "a-1",
		DocID:       "doc-1",
		DocumentRef: "/api/v1/documents/doc-1/download",
		Checksum:    "abc123",
		Params:      domain.PrintParams{Copies: 2, Orientation: "landscape"},
		Status:      domain.TaskStatusPending,
		SubmittedAt: now,
	}
	require.NoError(t, repo.Create(ctx, task))

	got, err := repo.GetByID(ctx, "t-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "u-1", got.UserID)
	assert.Equal(t, "d-1", got.DeviceID)
	assert.Equal(t, 2, got.Params.Copies)
	assert.Equal(t, "landscape", got.Params.Orientation)
	assert.Equal(t, domain.TaskStatusPending, got.Status)

	require.NoError(t, repo.MarkDispatched(ctx, "t-1"))
	require.NoError(t, repo.MarkStarted(ctx, "t-1"))
	started, err := repo.GetByID(ctx, "t-1")
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusRunning, started.Status)
	assert.False(t, started.StartedAt.IsZero())

	require.NoError(t, repo.MarkFinished(ctx, "t-1", domain.TaskStatusSuccess, "", ""))
	finished, err := repo.GetByID(ctx, "t-1")
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusSuccess, finished.Status)
	assert.False(t, finished.FinishedAt.IsZero())

	require.NoError(t, repo.UpdateStatus(ctx, "t-1", domain.TaskStatusFailed, 1, "PRINT_FAIL", "纸张卡住"))
	updated, err := repo.GetByID(ctx, "t-1")
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusFailed, updated.Status)
	assert.Equal(t, 1, updated.RetryCount)
	assert.Equal(t, "PRINT_FAIL", updated.ErrorCode)

	require.NoError(t, repo.Create(ctx, &domain.PrintTask{TaskID: "t-2", UserID: "u-1", DeviceID: "d-1", AgentID: "a-1", Status: domain.TaskStatusPending, SubmittedAt: time.Now().UTC()}))

	byUser, err := repo.ListByUser(ctx, "u-1", 10, 0)
	require.NoError(t, err)
	assert.Len(t, byUser, 2)

	byAgent, err := repo.ListByAgentAndStatus(ctx, "a-1", domain.TaskStatusPending)
	require.NoError(t, err)
	assert.Len(t, byAgent, 1)

	all, err := repo.ListAll(ctx, 10, 0)
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestDocumentRepoCRUD(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	userRepo := NewUserRepo(db)
	repo := NewDocumentRepo(db)
	ctx := context.Background()

	require.NoError(t, userRepo.Create(ctx, &domain.User{UserID: "u-1", Username: "alice", PasswordHash: "h", PasswordSalt: "", Role: domain.RoleUser, Status: domain.UserStatusActive}))

	doc := &domain.Document{
		DocID:       "doc-1",
		UserID:      "u-1",
		Filename:    "test.pdf",
		ContentType: "application/pdf",
		Size:        1024,
		Checksum:    "sha256-abc",
		StoragePath: "/data/docs/doc-1.bin",
		CleanupAt:   time.Now().UTC().Add(24 * time.Hour),
	}
	require.NoError(t, repo.Create(ctx, doc))

	got, err := repo.GetByID(ctx, "doc-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "test.pdf", got.Filename)
	assert.Equal(t, int64(1024), got.Size)

	list, err := repo.ListByUser(ctx, "u-1", 10, 0)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	expired, err := repo.ListExpired(ctx, time.Now().UTC().Add(48*time.Hour))
	require.NoError(t, err)
	assert.Len(t, expired, 1)

	require.NoError(t, repo.Delete(ctx, "doc-1"))
	deleted, err := repo.GetByID(ctx, "doc-1")
	require.NoError(t, err)
	assert.Nil(t, deleted)
}

func TestAuditRepoCRUD(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewAuditRepo(db)
	ctx := context.Background()

	a1 := &domain.AuditLog{AuditID: "au-1", UserID: "u-1", Action: "login", IP: "1.2.3.4", TS: time.Now().UTC()}
	a2 := &domain.AuditLog{AuditID: "au-2", UserID: "u-1", Action: "task_submit", Target: "t-1", IP: "1.2.3.4", TS: time.Now().UTC()}
	require.NoError(t, repo.Create(ctx, a1))
	require.NoError(t, repo.Create(ctx, a2))

	list, err := repo.List(ctx, "", time.Time{}, time.Time{}, 10, 0)
	require.NoError(t, err)
	assert.Len(t, list, 2)

	filtered, err := repo.List(ctx, "login", time.Time{}, time.Time{}, 10, 0)
	require.NoError(t, err)
	assert.Len(t, filtered, 1)

	require.NoError(t, repo.Cleanup(ctx, time.Now().UTC().Add(time.Second)))
	after, err := repo.List(ctx, "", time.Time{}, time.Time{}, 10, 0)
	require.NoError(t, err)
	assert.Len(t, after, 0)
}

func TestFactoryRepoHasDependents(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	factoryRepo := NewFactoryRepo(db)
	agentRepo := NewAgentRepo(db)
	ctx := context.Background()

	require.NoError(t, factoryRepo.Create(ctx, &domain.Factory{FactoryID: "f-1", Name: "F1"}))
	require.NoError(t, agentRepo.Create(ctx, &domain.Agent{AgentID: "a-1", Name: "A1", FactoryID: "f-1", DeviceTokenEnc: []byte("enc")}))

	has, err := factoryRepo.HasDependents(ctx, "f-1")
	require.NoError(t, err)
	assert.True(t, has)

	has, err = factoryRepo.HasDependents(ctx, "f-nonexistent")
	require.NoError(t, err)
	assert.False(t, has)
}