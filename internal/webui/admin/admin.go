package admin

import (
	"errors"
	"html/template"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/cloud-print/server/internal/auth"
)

type PageHandlers struct {
	tmpls  map[string]*template.Template
	jwtMgr *auth.JWTManager
	logger *zap.Logger
}

type PageHandlersConfig struct {
	TemplateFS fs.FS
	JWTMgr     *auth.JWTManager
	Logger     *zap.Logger
}

func NewPageHandlers(cfg PageHandlersConfig) (*PageHandlers, error) {
	tmpls, err := loadTemplates(cfg.TemplateFS)
	if err != nil {
		return nil, err
	}
	return &PageHandlers{tmpls: tmpls, jwtMgr: cfg.JWTMgr, logger: cfg.Logger}, nil
}

func loadTemplates(templateFS fs.FS) (map[string]*template.Template, error) {
	baseData, err := fs.ReadFile(templateFS, "templates/admin/base.html")
	if err != nil {
		return nil, err
	}
	funcMap := template.FuncMap{
		"hasPrefix": strings.HasPrefix,
		"lower":     strings.ToLower,
		"upper":     strings.ToUpper,
	}
	baseTmpl := template.New("base.html").Funcs(funcMap)
	if _, err := baseTmpl.Parse(string(baseData)); err != nil {
		return nil, err
	}

	out := make(map[string]*template.Template)
	entries, err := fs.ReadDir(templateFS, "templates/admin")
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "base.html" || !strings.HasSuffix(name, ".html") {
			continue
		}
		pageData, err := fs.ReadFile(templateFS, filepath.Join("templates/admin", name))
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
	if len(out) == 0 {
		return nil, errors.New("no admin page templates found")
	}
	return out, nil
}

func RegisterAdminPages(r chi.Router, handlers *PageHandlers) {
	r.Route("/admin", func(r chi.Router) {
		r.Use(auth.JWTMiddleware(handlers.jwtMgr))
		r.Use(auth.AdminOnlyMiddleware)

		r.Get("/", handlers.Dashboard)
		r.Get("/users", handlers.UsersPage)
		r.Get("/factories", handlers.FactoriesPage)
		r.Get("/agents", handlers.AgentsPage)
		r.Get("/devices", handlers.DevicesPage)
		r.Get("/audit", handlers.AuditPage)
	})
}

func (h *PageHandlers) render(w http.ResponseWriter, name string, data interface{}) {
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

func (h *PageHandlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	h.render(w, "dashboard.html", map[string]interface{}{
		"Title":  "仪表盘",
		"Active": "dashboard",
		"UserID": auth.UserIDFromCtx(r.Context()),
		"Role":   "ADMIN",
	})
}

func (h *PageHandlers) UsersPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, "users.html", map[string]interface{}{
		"Title":  "用户管理",
		"Active": "users",
		"UserID": auth.UserIDFromCtx(r.Context()),
		"Role":   "ADMIN",
	})
}

func (h *PageHandlers) FactoriesPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, "factories.html", map[string]interface{}{
		"Title":  "工厂管理",
		"Active": "factories",
		"UserID": auth.UserIDFromCtx(r.Context()),
		"Role":   "ADMIN",
	})
}

func (h *PageHandlers) AgentsPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, "agents.html", map[string]interface{}{
		"Title":  "Agent 管理",
		"Active": "agents",
		"UserID": auth.UserIDFromCtx(r.Context()),
		"Role":   "ADMIN",
	})
}

func (h *PageHandlers) DevicesPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, "devices.html", map[string]interface{}{
		"Title":  "设备管理",
		"Active": "devices",
		"UserID": auth.UserIDFromCtx(r.Context()),
		"Role":   "ADMIN",
	})
}

func (h *PageHandlers) AuditPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, "audit.html", map[string]interface{}{
		"Title":  "审计日志",
		"Active": "audit",
		"UserID": auth.UserIDFromCtx(r.Context()),
		"Role":   "ADMIN",
	})
}
