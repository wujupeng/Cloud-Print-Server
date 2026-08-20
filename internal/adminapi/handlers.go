package adminapi

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/cloud-print/server/internal/agentmanager"
	"github.com/cloud-print/server/internal/auth"
	"github.com/cloud-print/server/internal/devicemanager"
	"github.com/cloud-print/server/internal/domain"
	"github.com/cloud-print/server/internal/errs"
	"github.com/cloud-print/server/internal/observability"
	"github.com/cloud-print/server/internal/storage"
	"github.com/cloud-print/server/internal/wsshub"
)

type AdminHandlers struct {
	jwtMgr      *auth.JWTManager
	userRepo    *storage.UserRepo
	factoryRepo *storage.FactoryRepo
	agentRepo   *storage.AgentRepo
	agentMgr    *agentmanager.Manager
	deviceMgr   *devicemanager.Manager
	deviceRepo  *storage.DeviceRepo
	auditRepo   *storage.AuditRepo
	hub         *wsshub.Hub
	logger      *zap.Logger
	audit       *observability.AuditLogger
	bcryptCost  int
}

type AdminHandlersConfig struct {
	JWTMgr      *auth.JWTManager
	UserRepo    *storage.UserRepo
	FactoryRepo *storage.FactoryRepo
	AgentRepo   *storage.AgentRepo
	AgentMgr    *agentmanager.Manager
	DeviceMgr   *devicemanager.Manager
	DeviceRepo  *storage.DeviceRepo
	AuditRepo   *storage.AuditRepo
	Hub         *wsshub.Hub
	Logger      *zap.Logger
	Audit       *observability.AuditLogger
	BcryptCost  int
}

func NewAdminHandlers(cfg AdminHandlersConfig) *AdminHandlers {
	cost := cfg.BcryptCost
	if cost <= 0 {
		cost = 10
	}
	return &AdminHandlers{
		jwtMgr:      cfg.JWTMgr,
		userRepo:    cfg.UserRepo,
		factoryRepo: cfg.FactoryRepo,
		agentRepo:   cfg.AgentRepo,
		agentMgr:    cfg.AgentMgr,
		deviceMgr:   cfg.DeviceMgr,
		deviceRepo:  cfg.DeviceRepo,
		auditRepo:   cfg.AuditRepo,
		hub:         cfg.Hub,
		logger:      cfg.Logger,
		audit:       cfg.Audit,
		bcryptCost:  cost,
	}
}

type adminResponse struct {
	Code    string      `json:"code"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func writeAdminJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeAdminOK(w http.ResponseWriter, data interface{}) {
	writeAdminJSON(w, http.StatusOK, adminResponse{Code: "OK", Data: data})
}

func writeAdminCreated(w http.ResponseWriter, data interface{}) {
	writeAdminJSON(w, http.StatusCreated, adminResponse{Code: "OK", Data: data})
}

func writeAdminError(w http.ResponseWriter, status int, code, message string) {
	writeAdminJSON(w, status, adminResponse{Code: code, Message: message})
}

func writeAdminAPIError(w http.ResponseWriter, err error) {
	if err == nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "unknown error")
		return
	}
	var apiErr *errs.APIError
	if errors.As(err, &apiErr) {
		status := apiErr.HTTPStatus
		if status == 0 {
			status = http.StatusInternalServerError
		}
		writeAdminError(w, status, string(apiErr.Code), apiErr.Message)
		return
	}
	writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
}

func adminUserID(r *http.Request) string {
	return auth.UserIDFromCtx(r.Context())
}

func parseAdminPagination(r *http.Request) (int, int) {
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

type createUserRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Status      string `json:"status"`
}

type updateUserRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Status      string `json:"status"`
}

func (h *AdminHandlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.userRepo.List(r.Context())
	if err != nil {
		writeAdminAPIError(w, err)
		return
	}
	if users == nil {
		users = []*domain.User{}
	}
	writeAdminOK(w, users)
}

func (h *AdminHandlers) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAdminError(w, http.StatusBadRequest, "PARAM_INVALID", "invalid request body")
		return
	}
	if len(req.Username) < 3 || len(req.Password) < 6 {
		writeAdminError(w, http.StatusBadRequest, "PARAM_INVALID", "username>=3 and password>=6 required")
		return
	}

	ctx := r.Context()
	if existing, _ := h.userRepo.GetByUsername(ctx, req.Username); existing != nil {
		writeAdminError(w, http.StatusConflict, "USERNAME_EXISTS", "username already exists")
		return
	}

	hash, _, err := auth.HashPassword(req.Password, h.bcryptCost)
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "hash password failed")
		return
	}

	role := domain.RoleUser
	if strings.EqualFold(req.Role, "ADMIN") {
		role = domain.RoleAdmin
	}
	status := domain.UserStatusActive
	if strings.EqualFold(req.Status, "DISABLED") {
		status = domain.UserStatusDisabled
	}

	user := &domain.User{
		UserID:       uuid.NewString(),
		Username:     req.Username,
		PasswordHash: hash,
		Role:         role,
		Status:       status,
		DisplayName:  req.DisplayName,
	}
	if err := h.userRepo.Create(ctx, user); err != nil {
		h.logger.Error("admin create user failed", zap.Error(err))
		writeAdminAPIError(w, err)
		return
	}

	if h.audit != nil {
		h.audit.UserManage(adminUserID(r), "create", user.UserID, r.RemoteAddr)
	}
	writeAdminCreated(w, map[string]string{
		"user_id":  user.UserID,
		"username": user.Username,
		"role":     user.Role.String(),
	})
}

func (h *AdminHandlers) UpdateUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		writeAdminError(w, http.StatusBadRequest, "PARAM_INVALID", "user id required")
		return
	}
	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAdminError(w, http.StatusBadRequest, "PARAM_INVALID", "invalid request body")
		return
	}

	ctx := r.Context()
	user, err := h.userRepo.GetByID(ctx, userID)
	if err != nil {
		writeAdminAPIError(w, err)
		return
	}
	if user == nil {
		writeAdminError(w, http.StatusNotFound, "USER_NOT_FOUND", "user not found")
		return
	}

	if req.Username != "" {
		user.Username = req.Username
	}
	if req.Password != "" {
		if len(req.Password) < 6 {
			writeAdminError(w, http.StatusBadRequest, "PARAM_INVALID", "password>=6 required")
			return
		}
		hash, _, err := auth.HashPassword(req.Password, h.bcryptCost)
		if err != nil {
			writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "hash password failed")
			return
		}
		user.PasswordHash = hash
	}
	if req.DisplayName != "" {
		user.DisplayName = req.DisplayName
	}
	if req.Role != "" {
		if strings.EqualFold(req.Role, "ADMIN") {
			user.Role = domain.RoleAdmin
		} else {
			user.Role = domain.RoleUser
		}
	}
	if req.Status != "" {
		if strings.EqualFold(req.Status, "DISABLED") {
			user.Status = domain.UserStatusDisabled
		} else {
			user.Status = domain.UserStatusActive
		}
	}

	if err := h.userRepo.Update(ctx, user); err != nil {
		writeAdminAPIError(w, err)
		return
	}
	if h.audit != nil {
		h.audit.UserManage(adminUserID(r), "update", user.UserID, r.RemoteAddr)
	}
	writeAdminOK(w, map[string]string{
		"user_id":  user.UserID,
		"username": user.Username,
		"role":     user.Role.String(),
		"status":   user.Status.String(),
	})
}

func (h *AdminHandlers) DeleteUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		writeAdminError(w, http.StatusBadRequest, "PARAM_INVALID", "user id required")
		return
	}
	if userID == adminUserID(r) {
		writeAdminError(w, http.StatusConflict, "SELF_DELETE", "cannot delete self")
		return
	}
	if err := h.userRepo.Delete(r.Context(), userID); err != nil {
		writeAdminAPIError(w, err)
		return
	}
	if h.audit != nil {
		h.audit.UserManage(adminUserID(r), "delete", userID, r.RemoteAddr)
	}
	writeAdminOK(w, map[string]string{"user_id": userID})
}

type createFactoryRequest struct {
	Name     string `json:"name"`
	Code     string `json:"code"`
	Location string `json:"location"`
}

func (h *AdminHandlers) ListFactories(w http.ResponseWriter, r *http.Request) {
	factories, err := h.factoryRepo.List(r.Context())
	if err != nil {
		writeAdminAPIError(w, err)
		return
	}
	if factories == nil {
		factories = []*domain.Factory{}
	}
	writeAdminOK(w, factories)
}

func (h *AdminHandlers) CreateFactory(w http.ResponseWriter, r *http.Request) {
	var req createFactoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAdminError(w, http.StatusBadRequest, "PARAM_INVALID", "invalid request body")
		return
	}
	if req.Name == "" {
		writeAdminError(w, http.StatusBadRequest, "PARAM_INVALID", "factory name required")
		return
	}
	factory := &domain.Factory{
		FactoryID: uuid.NewString(),
		Name:      req.Name,
		Code:      req.Code,
		Location:  req.Location,
	}
	if err := h.factoryRepo.Create(r.Context(), factory); err != nil {
		writeAdminAPIError(w, err)
		return
	}
	if h.audit != nil {
		h.audit.Log(adminUserID(r), "factory_create", factory.FactoryID, "", r.RemoteAddr)
	}
	writeAdminCreated(w, factory)
}

type registerAgentRequest struct {
	Name      string `json:"name"`
	FactoryID string `json:"factory_id"`
	Version   string `json:"version"`
}

type registerAgentResponse struct {
	AgentID    string    `json:"agent_id"`
	Name       string    `json:"name"`
	FactoryID  string    `json:"factory_id"`
	Token      string    `json:"token"`
	EncToken   string    `json:"enc_token"`
	AssignedAt time.Time `json:"assigned_at"`
}

func (h *AdminHandlers) ListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := h.agentRepo.List(r.Context())
	if err != nil {
		writeAdminAPIError(w, err)
		return
	}
	if agents == nil {
		agents = []*domain.Agent{}
	}
	writeAdminOK(w, agents)
}

func (h *AdminHandlers) RegisterAgent(w http.ResponseWriter, r *http.Request) {
	var req registerAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAdminError(w, http.StatusBadRequest, "PARAM_INVALID", "invalid request body")
		return
	}
	if req.Name == "" || req.FactoryID == "" {
		writeAdminError(w, http.StatusBadRequest, "PARAM_INVALID", "name and factory_id required")
		return
	}

	agentID := uuid.NewString()
	token, encToken, err := h.agentMgr.GenerateAgentToken(agentID)
	if err != nil {
		h.logger.Error("generate agent token failed", zap.Error(err))
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "generate token failed")
		return
	}
	encBytes, err := hex.DecodeString(encToken)
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "decode token failed")
		return
	}

	agent := &domain.Agent{
		AgentID:        agentID,
		Name:           req.Name,
		FactoryID:      req.FactoryID,
		DeviceTokenEnc: encBytes,
		Version:        req.Version,
	}
	if err := h.agentRepo.Create(r.Context(), agent); err != nil {
		writeAdminAPIError(w, err)
		return
	}
	if h.audit != nil {
		h.audit.CredentialGen(adminUserID(r), agentID, r.RemoteAddr)
	}
	writeAdminCreated(w, registerAgentResponse{
		AgentID:    agentID,
		Name:       agent.Name,
		FactoryID:  agent.FactoryID,
		Token:      token,
		EncToken:   encToken,
		AssignedAt: time.Now().UTC(),
	})
}

type agentCredentialsResponse struct {
	AgentID  string `json:"agent_id"`
	Token    string `json:"token"`
	EncToken string `json:"enc_token"`
}

func (h *AdminHandlers) GetAgentCredentials(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")
	if agentID == "" {
		writeAdminError(w, http.StatusBadRequest, "PARAM_INVALID", "agent id required")
		return
	}
	agent, err := h.agentRepo.GetByID(r.Context(), agentID)
	if err != nil {
		writeAdminAPIError(w, err)
		return
	}
	if agent == nil {
		writeAdminError(w, http.StatusNotFound, "AGENT_NOT_FOUND", "agent not found")
		return
	}

	token, encToken, err := h.agentMgr.GenerateAgentToken(agentID)
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "generate token failed")
		return
	}
	encBytes, err := hex.DecodeString(encToken)
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "decode token failed")
		return
	}
	agent.DeviceTokenEnc = encBytes
	if err := h.agentRepo.Update(r.Context(), agent); err != nil {
		writeAdminAPIError(w, err)
		return
	}
	if h.audit != nil {
		h.audit.CredentialGen(adminUserID(r), agentID, r.RemoteAddr)
	}
	writeAdminOK(w, agentCredentialsResponse{
		AgentID:  agentID,
		Token:    token,
		EncToken: encToken,
	})
}

type addDeviceRequest struct {
	Name      string `json:"name"`
	IP        string `json:"ip"`
	Hostname  string `json:"hostname"`
	Model     string `json:"model"`
	Protocol  string `json:"protocol"`
	FactoryID string `json:"factory_id"`
	AgentID   string `json:"agent_id"`
	Port      int    `json:"port"`
}

type updateDeviceRequest struct {
	Name      string `json:"name"`
	IP        string `json:"ip"`
	Hostname  string `json:"hostname"`
	Model     string `json:"model"`
	Protocol  string `json:"protocol"`
	FactoryID string `json:"factory_id"`
	AgentID   string `json:"agent_id"`
	Port      int    `json:"port"`
	Status    string `json:"status"`
}

func (h *AdminHandlers) ListAllDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := h.deviceRepo.List(r.Context())
	if err != nil {
		writeAdminAPIError(w, err)
		return
	}
	if devices == nil {
		devices = []*domain.Device{}
	}
	writeAdminOK(w, devices)
}

func (h *AdminHandlers) AddDevice(w http.ResponseWriter, r *http.Request) {
	var req addDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAdminError(w, http.StatusBadRequest, "PARAM_INVALID", "invalid request body")
		return
	}
	if req.Name == "" || req.IP == "" || req.AgentID == "" {
		writeAdminError(w, http.StatusBadRequest, "PARAM_INVALID", "name, ip and agent_id required")
		return
	}
	protocol := domain.Protocol(req.Protocol)
	if protocol == "" {
		protocol = domain.ProtocolRAW
	}
	device := &domain.Device{
		DeviceID:  uuid.NewString(),
		Name:      req.Name,
		IP:        req.IP,
		Hostname:  req.Hostname,
		Model:     req.Model,
		Protocol:  protocol,
		FactoryID: req.FactoryID,
		AgentID:   req.AgentID,
		Port:      req.Port,
		Status:    domain.DeviceStatusOffline,
	}
	if err := h.deviceMgr.CreateDevice(r.Context(), device); err != nil {
		writeAdminAPIError(w, err)
		return
	}
	if h.audit != nil {
		h.audit.DeviceManage(adminUserID(r), "create", device.DeviceID, r.RemoteAddr)
	}
	writeAdminCreated(w, device)
}

func (h *AdminHandlers) UpdateDevice(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "id")
	if deviceID == "" {
		writeAdminError(w, http.StatusBadRequest, "PARAM_INVALID", "device id required")
		return
	}
	var req updateDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAdminError(w, http.StatusBadRequest, "PARAM_INVALID", "invalid request body")
		return
	}

	ctx := r.Context()
	device, err := h.deviceMgr.GetDevice(ctx, deviceID)
	if err != nil {
		writeAdminAPIError(w, err)
		return
	}
	if req.Name != "" {
		device.Name = req.Name
	}
	if req.IP != "" {
		device.IP = req.IP
	}
	if req.Hostname != "" {
		device.Hostname = req.Hostname
	}
	if req.Model != "" {
		device.Model = req.Model
	}
	if req.Protocol != "" {
		device.Protocol = domain.Protocol(req.Protocol)
	}
	if req.FactoryID != "" {
		device.FactoryID = req.FactoryID
	}
	if req.AgentID != "" {
		device.AgentID = req.AgentID
	}
	if req.Port > 0 {
		device.Port = req.Port
	}
	if req.Status != "" {
		device.Status = domain.DeviceStatus(req.Status)
	}

	if err := h.deviceMgr.UpdateDevice(ctx, device); err != nil {
		writeAdminAPIError(w, err)
		return
	}
	if h.audit != nil {
		h.audit.DeviceManage(adminUserID(r), "update", deviceID, r.RemoteAddr)
	}
	writeAdminOK(w, device)
}

func (h *AdminHandlers) DeleteDevice(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "id")
	if deviceID == "" {
		writeAdminError(w, http.StatusBadRequest, "PARAM_INVALID", "device id required")
		return
	}
	if err := h.deviceMgr.DeleteDevice(r.Context(), deviceID); err != nil {
		writeAdminAPIError(w, err)
		return
	}
	if h.audit != nil {
		h.audit.DeviceManage(adminUserID(r), "delete", deviceID, r.RemoteAddr)
	}
	writeAdminOK(w, map[string]string{"device_id": deviceID})
}

type configUpdateRequest struct {
	ConfigVersion int                    `json:"config_version"`
	Patch         map[string]interface{} `json:"patch"`
	Reason        string                 `json:"reason,omitempty"`
}

func (h *AdminHandlers) ConfigUpdate(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")
	if agentID == "" {
		writeAdminError(w, http.StatusBadRequest, "PARAM_INVALID", "agent id required")
		return
	}
	var req configUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAdminError(w, http.StatusBadRequest, "PARAM_INVALID", "invalid request body")
		return
	}
	if req.ConfigVersion <= 0 || req.Patch == nil {
		writeAdminError(w, http.StatusBadRequest, "PARAM_INVALID", "config_version and patch required")
		return
	}
	if !h.agentMgr.IsOnline(agentID) {
		writeAdminError(w, http.StatusConflict, "AGENT_OFFLINE_UPGRADE", "agent is offline")
		return
	}

	payload := domain.ConfigUpdatePayload{
		ConfigVersion: req.ConfigVersion,
		Patch:         req.Patch,
	}
	if err := h.hub.SendConfigUpdate(agentID, payload); err != nil {
		writeAdminAPIError(w, err)
		return
	}
	if h.audit != nil {
		h.audit.ConfigChange(adminUserID(r), "agent_id="+agentID+", reason="+req.Reason, r.RemoteAddr)
	}
	writeAdminOK(w, map[string]interface{}{
		"agent_id":       agentID,
		"config_version": req.ConfigVersion,
		"dispatched":     true,
	})
}

func (h *AdminHandlers) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	limit, offset := parseAdminPagination(r)
	action := r.URL.Query().Get("action")
	var start, end time.Time
	if v := r.URL.Query().Get("start"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			start = t
		}
	}
	if v := r.URL.Query().Get("end"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			end = t
		}
	}
	logs, err := h.auditRepo.List(r.Context(), action, start, end, limit, offset)
	if err != nil {
		writeAdminAPIError(w, err)
		return
	}
	if logs == nil {
		logs = []*domain.AuditLog{}
	}
	writeAdminOK(w, logs)
}
