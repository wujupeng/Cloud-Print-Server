package restapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/cloud-print/server/internal/auth"
	"github.com/cloud-print/server/internal/devicemanager"
	"github.com/cloud-print/server/internal/docstore"
	"github.com/cloud-print/server/internal/domain"
	"github.com/cloud-print/server/internal/observability"
	"github.com/cloud-print/server/internal/storage"
	"github.com/cloud-print/server/internal/taskmanager"
)

type Handlers struct {
	jwtMgr     *auth.JWTManager
	userRepo   *storage.UserRepo
	docRepo    *storage.DocumentRepo
	taskMgr    *taskmanager.TaskManager
	deviceMgr  *devicemanager.Manager
	docStore   *docstore.Store
	sseHub     *SSEHub
	logger     *zap.Logger
	audit      *observability.AuditLogger
	bcryptCost int
	maxDocSize int64
	docRetention time.Duration
}

type HandlersConfig struct {
	JWTMgr        *auth.JWTManager
	UserRepo      *storage.UserRepo
	DocRepo       *storage.DocumentRepo
	TaskMgr       *taskmanager.TaskManager
	DeviceMgr     *devicemanager.Manager
	DocStore      *docstore.Store
	SSEHub        *SSEHub
	Logger        *zap.Logger
	Audit         *observability.AuditLogger
	BcryptCost    int
	MaxDocSizeMB  int
	DocRetentionHours int
}

func NewHandlers(cfg HandlersConfig) *Handlers {
	maxSize := int64(cfg.MaxDocSizeMB) * 1024 * 1024
	if maxSize <= 0 {
		maxSize = 50 * 1024 * 1024
	}
	cost := cfg.BcryptCost
	if cost <= 0 {
		cost = 10
	}
	retention := time.Duration(cfg.DocRetentionHours) * time.Hour
	if retention <= 0 {
		retention = 24 * time.Hour
	}
	return &Handlers{
		jwtMgr:       cfg.JWTMgr,
		userRepo:     cfg.UserRepo,
		docRepo:      cfg.DocRepo,
		taskMgr:      cfg.TaskMgr,
		deviceMgr:    cfg.DeviceMgr,
		docStore:     cfg.DocStore,
		sseHub:       cfg.SSEHub,
		logger:       cfg.Logger,
		audit:        cfg.Audit,
		bcryptCost:   cost,
		maxDocSize:   maxSize,
		docRetention: retention,
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	Username  string    `json:"username"`
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "PARAM_INVALID", "invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "PARAM_INVALID", "username and password required")
		return
	}

	ctx := r.Context()
	user, err := h.userRepo.GetByUsername(ctx, req.Username)
	if err != nil {
		h.logger.Error("login query user failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	if user == nil {
		writeError(w, http.StatusUnauthorized, "CREDENTIAL_INVALID", "invalid username or password")
		return
	}
	if user.Status != domain.UserStatusActive {
		writeError(w, http.StatusForbidden, "ACCOUNT_DISABLED", "account is disabled")
		return
	}
	if !auth.VerifyPassword(req.Password, user.PasswordHash, user.PasswordSalt) {
		writeError(w, http.StatusUnauthorized, "CREDENTIAL_INVALID", "invalid username or password")
		return
	}

	token, expiresAt, err := h.jwtMgr.IssueToken(user.UserID, user.Role.String())
	if err != nil {
		h.logger.Error("issue token failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}

	if h.audit != nil {
		h.audit.Login(user.UserID, r.RemoteAddr)
	}
	writeOK(w, loginResponse{
		Token:     token,
		ExpiresAt: expiresAt,
		UserID:    user.UserID,
		Role:      user.Role.String(),
		Username:  user.Username,
	})
}

type registerRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "PARAM_INVALID", "invalid request body")
		return
	}
	if len(req.Username) < 3 || len(req.Password) < 6 {
		writeError(w, http.StatusBadRequest, "PARAM_INVALID", "username>=3 and password>=6 required")
		return
	}

	ctx := r.Context()
	if existing, _ := h.userRepo.GetByUsername(ctx, req.Username); existing != nil {
		writeError(w, http.StatusConflict, "USERNAME_EXISTS", "username already exists")
		return
	}

	hash, _, err := auth.HashPassword(req.Password, h.bcryptCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "hash password failed")
		return
	}
	user := &domain.User{
		UserID:      uuid.NewString(),
		Username:    req.Username,
		PasswordHash: hash,
		Role:        domain.RoleUser,
		Status:      domain.UserStatusActive,
		DisplayName: req.DisplayName,
	}
	if err := h.userRepo.Create(ctx, user); err != nil {
		h.logger.Error("create user failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "create user failed")
		return
	}
	writeCreated(w, map[string]string{
		"user_id":  user.UserID,
		"username": user.Username,
	})
}

func (h *Handlers) ListDevices(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "AUTH_INVALID", "missing user")
		return
	}
	ctx := r.Context()
	devices, err := h.deviceMgr.ListDevices(ctx, userID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if devices == nil {
		devices = []*domain.Device{}
	}
	writeOK(w, devices)
}

type uploadResponse struct {
	DocID    string `json:"doc_id"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum"`
}

func (h *Handlers) UploadDocument(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "AUTH_INVALID", "missing user")
		return
	}

	if err := r.ParseMultipartForm(h.maxDocSize); err != nil {
		if errors.Is(err, http.ErrMissingBoundary) {
			writeError(w, http.StatusBadRequest, "PARAM_INVALID", "multipart form required")
			return
		}
		writeError(w, http.StatusRequestEntityTooLarge, "DOC_TOO_LARGE", "document too large")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "PARAM_INVALID", "file field required")
		return
	}
	defer file.Close()

	if header.Size > h.maxDocSize {
		writeError(w, http.StatusRequestEntityTooLarge, "DOC_TOO_LARGE", "document too large")
		return
	}

	docID := uuid.NewString()
	if err := h.docStore.Save(docID, file); err != nil {
		h.logger.Error("save document failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "STORAGE_IO", "save document failed")
		return
	}

	checksum, _ := h.docStore.CalcChecksum(docID)
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	doc := &domain.Document{
		DocID:       docID,
		UserID:      userID,
		Filename:    header.Filename,
		ContentType: contentType,
		Size:        header.Size,
		Checksum:    checksum,
		StoragePath: h.docStore.GetPath(docID),
		CleanupAt:   time.Now().UTC().Add(h.docRetention),
	}
	if err := h.docRepo.Create(r.Context(), doc); err != nil {
		_ = h.docStore.Delete(docID)
		h.logger.Error("create document record failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "create document record failed")
		return
	}

	writeCreated(w, uploadResponse{
		DocID:    docID,
		Filename: header.Filename,
		Size:     header.Size,
		Checksum: checksum,
	})
}

type createTaskRequest struct {
	DeviceID string                 `json:"device_id"`
	DocID    string                 `json:"doc_id"`
	Copies   int                    `json:"copies"`
	Orientation string              `json:"orientation"`
	Extra    map[string]string      `json:"extra"`
}

func (h *Handlers) CreateTask(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "AUTH_INVALID", "missing user")
		return
	}
	var req createTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "PARAM_INVALID", "invalid request body")
		return
	}
	if req.DeviceID == "" || req.DocID == "" {
		writeError(w, http.StatusBadRequest, "PARAM_INVALID", "device_id and doc_id required")
		return
	}

	ctx := r.Context()
	doc, err := h.docRepo.GetByID(ctx, req.DocID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if doc == nil || doc.UserID != userID {
		writeError(w, http.StatusNotFound, "DOC_NOT_FOUND", "document not found")
		return
	}

	params := domain.PrintParams{
		Copies:      req.Copies,
		Orientation: req.Orientation,
		Extra:       req.Extra,
	}
	task, err := h.taskMgr.CreateTask(ctx, userID, req.DeviceID, req.DocID, params)
	if err != nil {
		writeAPIError(w, err)
		return
	}

	go func(taskID string) {
		dispatchCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := h.taskMgr.DispatchTask(dispatchCtx, taskID); err != nil {
			h.logger.Warn("auto dispatch failed", zap.String("task_id", taskID), zap.Error(err))
		}
	}(task.TaskID)

	writeCreated(w, task)
}

func (h *Handlers) ListTasks(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "AUTH_INVALID", "missing user")
		return
	}
	limit, offset := parsePagination(r)
	tasks, err := h.taskMgr.ListTasks(r.Context(), userID, limit, offset)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if tasks == nil {
		tasks = []*domain.PrintTask{}
	}
	writeOK(w, tasks)
}

func (h *Handlers) GetTask(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "AUTH_INVALID", "missing user")
		return
	}
	taskID := chi.URLParam(r, "id")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "PARAM_INVALID", "task id required")
		return
	}
	task, err := h.taskMgr.GetTask(r.Context(), taskID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if task.UserID != userID && !auth.IsAdminFromCtx(r.Context()) {
		writeError(w, http.StatusForbidden, "NO_PERMISSION", "task does not belong to user")
		return
	}
	writeOK(w, task)
}

func (h *Handlers) CancelTask(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "AUTH_INVALID", "missing user")
		return
	}
	taskID := chi.URLParam(r, "id")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "PARAM_INVALID", "task id required")
		return
	}
	if err := h.taskMgr.CancelTask(r.Context(), taskID, userID); err != nil {
		writeAPIError(w, err)
		return
	}
	writeOK(w, map[string]string{"task_id": taskID, "status": "CANCELLED"})
}

func (h *Handlers) Events(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "AUTH_INVALID", "missing user")
		return
	}
	h.sseHub.ServeSSE(w, r, userID)
}

func (h *Handlers) DownloadDocument(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "AUTH_INVALID", "missing user")
		return
	}
	docID := chi.URLParam(r, "id")
	if docID == "" {
		writeError(w, http.StatusBadRequest, "PARAM_INVALID", "doc id required")
		return
	}
	doc, err := h.docRepo.GetByID(r.Context(), docID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if doc == nil {
		writeError(w, http.StatusNotFound, "DOC_NOT_FOUND", "document not found")
		return
	}
	if doc.UserID != userID && !auth.IsAdminFromCtx(r.Context()) {
		writeError(w, http.StatusForbidden, "NO_PERMISSION", "document not accessible")
		return
	}
	rc, err := h.docStore.Load(docID)
	if err != nil {
		writeError(w, http.StatusNotFound, "DOC_NOT_FOUND", "document file not found")
		return
	}
	defer rc.Close()
	contentType := doc.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, sanitizeFilename(doc.Filename)))
	w.Header().Set("X-Checksum", doc.Checksum)
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

func parsePagination(r *http.Request) (int, int) {
	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := parseIntStrict(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := parseIntStrict(v); err == nil && n >= 0 {
			offset = n
		}
	}
	if limit > 500 {
		limit = 500
	}
	return limit, offset
}

func parseIntStrict(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, `"`, "'")
	name = strings.ReplaceAll(name, `\`, "_")
	return name
}
