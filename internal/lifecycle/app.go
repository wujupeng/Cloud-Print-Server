package lifecycle

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/cloud-print/server/internal/agentmanager"
	"github.com/cloud-print/server/internal/adminapi"
	"github.com/cloud-print/server/internal/auth"
	"github.com/cloud-print/server/internal/config"
	"github.com/cloud-print/server/internal/devicemanager"
	"github.com/cloud-print/server/internal/docstore"
	"github.com/cloud-print/server/internal/httpserver"
	"github.com/cloud-print/server/internal/observability"
	"github.com/cloud-print/server/internal/restapi"
	"github.com/cloud-print/server/internal/storage"
	"github.com/cloud-print/server/internal/taskmanager"
	webuiadmin "github.com/cloud-print/server/internal/webui/admin"
	"github.com/cloud-print/server/internal/webui"
	"github.com/cloud-print/server/internal/wsshub"
)

type App struct {
	cfg         *config.ServerConfig
	logger      *zap.Logger
	auditLogger *observability.AuditLogger

	db *storage.DB

	userRepo    *storage.UserRepo
	factoryRepo *storage.FactoryRepo
	agentRepo   *storage.AgentRepo
	deviceRepo  *storage.DeviceRepo
	taskRepo    *storage.TaskRepo
	docRepo     *storage.DocumentRepo
	auditRepo   *storage.AuditRepo

	jwtMgr  *auth.JWTManager
	credMgr *agentmanager.CredentialManager

	agentMgr  *agentmanager.Manager
	deviceMgr *devicemanager.Manager
	docStore  *docstore.Store
	taskMgr   *taskmanager.TaskManager
	hub       *wsshub.Hub
	sseHub    *restapi.SSEHub

	server *httpserver.Server

	cancel context.CancelFunc
}

func Run(configPath string) error {
	app, err := buildApp(configPath)
	if err != nil {
		return err
	}
	return app.serve()
}

func buildApp(configPath string) (*App, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	if err := config.Validate(cfg); err != nil {
		return nil, err
	}

	logger, err := observability.NewLogger(cfg.Log.Dir, cfg.Log.Level)
	if err != nil {
		return nil, err
	}

	auditRaw, err := observability.NewAuditLogger(cfg.Log.Dir, cfg.Audit.RetentionDays)
	if err != nil {
		return nil, err
	}
	auditLogger := observability.NewAuditLoggerWrapper(auditRaw)

	db, err := storage.Open(cfg.DB.Path)
	if err != nil {
		return nil, err
	}

	migrationsDir := resolveMigrationsDir()
	if migrationsDir != "" {
		if err := storage.RunMigrations(cfg.DB.Path, migrationsDir); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("run migrations: %w", err)
		}
		logger.Info("数据库迁移完成", zap.String("dir", migrationsDir))
	} else {
		logger.Warn("未找到迁移目录，跳过迁移")
	}

	app := &App{
		cfg:         cfg,
		logger:      logger,
		auditLogger: auditLogger,
		db:          db,
	}

	app.buildRepos()
	app.buildCore()
	if err := app.buildServer(); err != nil {
		return nil, err
	}
	return app, nil
}

func (a *App) buildRepos() {
	a.userRepo = storage.NewUserRepo(a.db)
	a.factoryRepo = storage.NewFactoryRepo(a.db)
	a.agentRepo = storage.NewAgentRepo(a.db)
	a.deviceRepo = storage.NewDeviceRepo(a.db)
	a.taskRepo = storage.NewTaskRepo(a.db)
	a.docRepo = storage.NewDocumentRepo(a.db)
	a.auditRepo = storage.NewAuditRepo(a.db)
}

func (a *App) buildCore() {
	a.jwtMgr = auth.NewJWTManager(a.cfg.Auth.JWTSecret, a.cfg.Auth.JWTTTLHours)
	a.credMgr = agentmanager.NewCredentialManager(a.cfg.Auth.MasterKey)

	agentRepoBridge := agentmanager.NewRepo(a.agentRepo)
	a.agentMgr = agentmanager.NewManager(agentRepoBridge, a.credMgr, a.logger)

	a.deviceMgr = devicemanager.NewManager(a.deviceRepo, a.logger)
	a.deviceMgr.SetUserRepo(a.userRepo)

	a.docStore = docstore.NewStore(filepath.Join(a.cfg.Storage.DataDir, "docs"), a.logger)

	a.taskMgr = taskmanager.NewTaskManager(a.taskRepo, a.docStore, nil, a.logger, a.auditLogger)
	a.taskMgr.SetDeviceRepo(a.deviceRepo)

	a.hub = wsshub.NewHub(a.agentMgr, a.taskMgr, a.deviceMgr, a.logger, a.auditLogger)
	a.taskMgr.SetHub(a.hub)

	a.sseHub = restapi.NewSSEHub(a.logger)
	a.taskMgr.SetStatusNotifier(a.sseHub)
}

func (a *App) buildServer() error {
	a.server = httpserver.NewServer(a.cfg, a.logger, a.auditLogger)
	router := a.server.Router()

	restHandlers := restapi.NewHandlers(restapi.HandlersConfig{
		JWTMgr:            a.jwtMgr,
		UserRepo:          a.userRepo,
		DocRepo:           a.docRepo,
		TaskMgr:           a.taskMgr,
		DeviceMgr:         a.deviceMgr,
		DocStore:          a.docStore,
		SSEHub:            a.sseHub,
		Logger:            a.logger,
		Audit:             a.auditLogger,
		BcryptCost:        a.cfg.Auth.BcryptCost,
		MaxDocSizeMB:      a.cfg.Upload.MaxSizeMB,
		DocRetentionHours: a.cfg.Storage.DocRetentionHours,
	})
	restapi.RegisterRoutes(router, restHandlers)

	adminHandlers := adminapi.NewAdminHandlers(adminapi.AdminHandlersConfig{
		JWTMgr:      a.jwtMgr,
		UserRepo:    a.userRepo,
		FactoryRepo: a.factoryRepo,
		AgentRepo:   a.agentRepo,
		AgentMgr:    a.agentMgr,
		DeviceMgr:   a.deviceMgr,
		DeviceRepo:  a.deviceRepo,
		AuditRepo:   a.auditRepo,
		Hub:         a.hub,
		Logger:      a.logger,
		Audit:       a.auditLogger,
		BcryptCost:  a.cfg.Auth.BcryptCost,
	})
	adminapi.RegisterAdminRoutes(router, adminHandlers)

	router.Get(a.cfg.WSS.Path, a.hub.HandleAgentWS)

	if err := a.registerWebUI(router); err != nil {
		a.logger.Warn("webui 注册失败，跳过", zap.Error(err))
	}

	return nil
}

func (a *App) registerWebUI(router chi.Router) error {
	webDir := resolveWebDir()
	if webDir == "" {
		return nil
	}
	if _, err := os.Stat(filepath.Join(webDir, "templates")); err != nil {
		return nil
	}
	templateFS := os.DirFS(webDir)

	uiHandlers, err := webui.NewHandlers(webui.HandlersConfig{
		TemplateFS: templateFS,
		JWTMgr:     a.jwtMgr,
		UserRepo:   a.userRepo,
		TaskMgr:    a.taskMgr,
		DeviceMgr:  a.deviceMgr,
		Logger:     a.logger,
		Audit:      a.auditLogger,
	})
	if err != nil {
		return err
	}
	webui.RegisterPages(router, uiHandlers)

	adminPageHandlers, err := webuiadmin.NewPageHandlers(webuiadmin.PageHandlersConfig{
		TemplateFS: templateFS,
		JWTMgr:     a.jwtMgr,
		Logger:     a.logger,
	})
	if err != nil {
		return err
	}
	webuiadmin.RegisterAdminPages(router, adminPageHandlers)

	staticDir := filepath.Join(webDir, "static")
	if _, err := os.Stat(staticDir); err == nil {
		fileServer := http.FileServer(http.Dir(staticDir))
		router.Handle("/static/*", http.StripPrefix("/static/", fileServer))
	}
	return nil
}

func resolveWebDir() string {
	candidates := []string{
		"web",
		"/usr/share/cloud-print-server/web",
		"/etc/cloud-print-server/web",
	}
	exe, err := os.Executable()
	if err == nil {
		candidates = append([]string{
			filepath.Join(filepath.Dir(exe), "web"),
			filepath.Join(filepath.Dir(exe), "..", "share", "cloud-print-server", "web"),
		}, candidates...)
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	return ""
}

func resolveMigrationsDir() string {
	candidates := []string{
		"migrations",
		"/etc/cloud-print-server/migrations",
		"/usr/share/cloud-print-server/migrations",
	}
	exe, err := os.Executable()
	if err == nil {
		candidates = append([]string{
			filepath.Join(filepath.Dir(exe), "migrations"),
			filepath.Join(filepath.Dir(exe), "..", "share", "cloud-print-server", "migrations"),
		}, candidates...)
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	return ""
}

func (a *App) serve() error {
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel

	go a.hub.StartHeartbeatChecker(ctx)
	go StartWatchdog(ctx, a.logger)

	notifyReady(a.logger)

	errCh := make(chan error, 1)
	go func() {
		errCh <- a.server.Start(ctx)
	}()

	sigCh := WaitForSignal()
	select {
	case err := <-errCh:
		a.logger.Error("server exited", zap.Error(err))
		_ = a.Shutdown()
		return err
	case sig := <-sigCh:
		a.logger.Info("收到信号，开始优雅关停", zap.String("signal", sig.String()))
	}

	return a.Shutdown()
}

func (a *App) Shutdown() error {
	if a.cancel != nil {
		a.cancel()
	}
	var firstErr error
	if a.server != nil {
		if err := a.server.Stop(); err != nil {
			a.logger.Warn("http server stop failed", zap.Error(err))
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if a.db != nil {
		if err := a.db.Close(); err != nil {
			a.logger.Warn("db close failed", zap.Error(err))
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if a.logger != nil {
		_ = a.logger.Sync()
	}
	return firstErr
}

func notifyReady(logger *zap.Logger) {
	if !isSystemd() {
		return
	}
	if err := NotifyReady(); err != nil {
		logger.Warn("systemd notify READY=1 失败", zap.Error(err))
		return
	}
	logger.Info("已通知 systemd READY=1")
}

func isSystemd() bool {
	return os.Getenv("INVOCATION_ID") != "" || os.Getenv("JOURNAL_STREAM") != ""
}

