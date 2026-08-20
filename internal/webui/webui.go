package webui

import (
	"encoding/json"
	"fmt"

	"html/template"
	"io/fs"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/cloud-print/server/internal/auth"
	"github.com/cloud-print/server/internal/devicemanager"
	"github.com/cloud-print/server/internal/domain"
	"github.com/cloud-print/server/internal/observability"
	"github.com/cloud-print/server/internal/storage"
	"github.com/cloud-print/server/internal/taskmanager"
)

type Handlers struct {
	tmpls     map[string]*template.Template
	jwtMgr    *auth.JWTManager
	userRepo  *storage.UserRepo
	taskMgr   *taskmanager.TaskManager
	deviceMgr *devicemanager.Manager
	logger    *zap.Logger
	audit     *observability.AuditLogger
}

type HandlersConfig struct {
	TemplateFS fs.FS
	JWTMgr     *auth.JWTManager
	UserRepo   *storage.UserRepo
	TaskMgr    *taskmanager.TaskManager
	DeviceMgr  *devicemanager.Manager
	Logger     *zap.Logger
	Audit      *observability.AuditLogger
}

func NewHandlers(cfg HandlersConfig) (*Handlers, error) {
	tmpls, err := loadTemplates(cfg.TemplateFS)
	if err != nil {
		return nil, err
	}
	return &Handlers{
		tmpls:     tmpls,
		jwtMgr:    cfg.JWTMgr,
		userRepo:  cfg.UserRepo,
		taskMgr:   cfg.TaskMgr,
		deviceMgr: cfg.DeviceMgr,
		logger:    cfg.Logger,
		audit:     cfg.Audit,
	}, nil
}

func loadTemplates(templateFS fs.FS) (map[string]*template.Template, error) {
	baseData, err := fs.ReadFile(templateFS, "templates/base.html")
	if err != nil {
		return nil, err
	}
	funcMap := template.FuncMap{
		"hasPrefix":  strings.HasPrefix,
		"lower":      func(v interface{}) string { return strings.ToLower(fmt.Sprintf("%v", v)) },
		"upper":      func(v interface{}) string { return strings.ToUpper(fmt.Sprintf("%v", v)) },
		"formatTime": formatTime,
	}
	baseTmpl := template.New("base.html").Funcs(funcMap)
	if _, err := baseTmpl.Parse(string(baseData)); err != nil {
		return nil, err
	}

	out := make(map[string]*template.Template)
	pages := []string{"login.html", "dashboard.html", "tasks.html", "task_new.html", "devices.html"}
	for _, name := range pages {
		pageData, err := fs.ReadFile(templateFS, filepath.Join("templates", name))
		if err != nil {
			return nil, err
		}
		clone, err := baseTmpl.Clone()
		if err != nil {
			return nil, err
		}
		pageTmpl, err := clone.New(name).Parse(string(pageData))
		if err != nil {
			return nil, err
		}
		out[name] = pageTmpl
	}
	return out, nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}

func RegisterPages(r chi.Router, handlers *Handlers) {
	r.Get("/", handlers.Index)
	r.Route("/login", func(r chi.Router) {
		r.Get("/", handlers.LoginPage)
		r.Post("/", handlers.LoginSubmit)
	})
	r.Group(func(r chi.Router) {
		r.Use(auth.JWTMiddleware(handlers.jwtMgr))
		r.Get("/dashboard", handlers.DashboardPage)
		r.Get("/tasks", handlers.TasksPage)
		r.Get("/tasks/new", handlers.TaskNewPage)
		r.Get("/devices", handlers.DevicesPage)
	})
}

func (h *Handlers) render(w http.ResponseWriter, name string, data interface{}) {
	tmpl, ok := h.tmpls[name]
	if !ok {
		h.logger.Error("template not found", zap.String("name", name))
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		h.logger.Error("render template failed", zap.String("name", name), zap.Error(err))
		http.Error(w, "render failed", http.StatusInternalServerError)
		return
	}
}

func (h *Handlers) renderLogin(w http.ResponseWriter, errMsg string) {
	h.render(w, "login.html", map[string]interface{}{
		"Title":  "登录",
		"Active": "login",
		"Error":  errMsg,
	})
}

func (h *Handlers) Index(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("token")
	if err != nil || cookie.Value == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if _, err := h.jwtMgr.ParseToken(cookie.Value); err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (h *Handlers) LoginPage(w http.ResponseWriter, r *http.Request) {
	h.renderLogin(w, "")
}

func (h *Handlers) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderLogin(w, "表单解析失败")
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	if username == "" || password == "" {
		h.renderLogin(w, "用户名和密码不能为空")
		return
	}

	ctx := r.Context()
	user, err := h.userRepo.GetByUsername(ctx, username)
	if err != nil {
		h.logger.Error("login query user failed", zap.Error(err))
		h.renderLogin(w, "内部错误")
		return
	}
	if user == nil || !auth.VerifyPassword(password, user.PasswordHash, user.PasswordSalt) {
		h.renderLogin(w, "用户名或密码错误")
		return
	}
	if user.Status != domain.UserStatusActive {
		h.renderLogin(w, "账号已被禁用")
		return
	}

	token, expiresAt, err := h.jwtMgr.IssueToken(user.UserID, user.Role.String())
	if err != nil {
		h.logger.Error("issue token failed", zap.Error(err))
		h.renderLogin(w, "内部错误")
		return
	}
	if h.audit != nil {
		h.audit.Login(user.UserID, r.RemoteAddr)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (h *Handlers) DashboardPage(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	ctx := r.Context()
	user, _ := h.userRepo.GetByID(ctx, userID)
	devices, _ := h.deviceMgr.ListDevices(ctx, userID)
	if devices == nil {
		devices = []*domain.Device{}
	}
	tasks, _ := h.taskMgr.ListTasks(ctx, userID, 10, 0)
	if tasks == nil {
		tasks = []*domain.PrintTask{}
	}
	h.render(w, "dashboard.html", map[string]interface{}{
		"Title":   "仪表盘",
		"Active":  "dashboard",
		"UserID":  userID,
		"Username": usernameOf(user),
		"Role":    roleOf(user),
		"Devices": devices,
		"Tasks":   tasks,
	})
}

func (h *Handlers) TasksPage(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	ctx := r.Context()
	user, _ := h.userRepo.GetByID(ctx, userID)
	limit, offset := parsePagination(r)
	tasks, _ := h.taskMgr.ListTasks(ctx, userID, limit, offset)
	if tasks == nil {
		tasks = []*domain.PrintTask{}
	}
	statusFilter := r.URL.Query().Get("status")
	h.render(w, "tasks.html", map[string]interface{}{
		"Title":       "任务列表",
		"Active":      "tasks",
		"UserID":      userID,
		"Username":    usernameOf(user),
		"Role":        roleOf(user),
		"Tasks":       tasks,
		"StatusFilter": statusFilter,
	})
}

func (h *Handlers) TaskNewPage(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	ctx := r.Context()
	user, _ := h.userRepo.GetByID(ctx, userID)
	devices, _ := h.deviceMgr.ListDevices(ctx, userID)
	if devices == nil {
		devices = []*domain.Device{}
	}
	h.render(w, "task_new.html", map[string]interface{}{
		"Title":    "新建任务",
		"Active":   "tasks",
		"UserID":   userID,
		"Username": usernameOf(user),
		"Role":     roleOf(user),
		"Devices":  devices,
	})
}

func (h *Handlers) DevicesPage(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	ctx := r.Context()
	user, _ := h.userRepo.GetByID(ctx, userID)
	devices, _ := h.deviceMgr.ListDevices(ctx, userID)
	if devices == nil {
		devices = []*domain.Device{}
	}
	groups := groupDevicesByFactory(devices)
	h.render(w, "devices.html", map[string]interface{}{
		"Title":    "设备列表",
		"Active":   "devices",
		"UserID":   userID,
		"Username": usernameOf(user),
		"Role":     roleOf(user),
		"Groups":   groups,
	})
}

func parsePagination(r *http.Request) (int, int) {
	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	if limit > 500 {
		limit = 500
	}
	return limit, offset
}

func usernameOf(user *domain.User) string {
	if user == nil {
		return ""
	}
	if user.DisplayName != "" {
		return user.DisplayName
	}
	return user.Username
}

func roleOf(user *domain.User) string {
	if user == nil {
		return ""
	}
	return user.Role.String()
}

type deviceGroup struct {
	FactoryID string
	Devices   []*domain.Device
}

func groupDevicesByFactory(devices []*domain.Device) []deviceGroup {
	groupMap := make(map[string][]*domain.Device)
	order := make([]string, 0)
	for _, d := range devices {
		key := d.FactoryID
		if _, ok := groupMap[key]; !ok {
			order = append(order, key)
		}
		groupMap[key] = append(groupMap[key], d)
	}
	out := make([]deviceGroup, 0, len(order))
	for _, key := range order {
		out = append(out, deviceGroup{FactoryID: key, Devices: groupMap[key]})
	}
	return out
}

type loginAPIRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginAPIResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	Username  string    `json:"username"`
}

func (h *Handlers) LoginAPI(w http.ResponseWriter, r *http.Request) {
	var req loginAPIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password required"})
		return
	}
	ctx := r.Context()
	user, err := h.userRepo.GetByUsername(ctx, req.Username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if user == nil || !auth.VerifyPassword(req.Password, user.PasswordHash, user.PasswordSalt) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	if user.Status != domain.UserStatusActive {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "account disabled"})
		return
	}
	token, expiresAt, err := h.jwtMgr.IssueToken(user.UserID, user.Role.String())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "issue token failed"})
		return
	}
	if h.audit != nil {
		h.audit.Login(user.UserID, r.RemoteAddr)
	}
	writeJSON(w, http.StatusOK, loginAPIResponse{
		Token:     token,
		ExpiresAt: expiresAt,
		UserID:    user.UserID,
		Role:      user.Role.String(),
		Username:  user.Username,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
