package observability

import (
	"time"

	"go.uber.org/zap"
)

type AuditLogger struct {
	logger *zap.Logger
}

func NewAuditLoggerWrapper(logger *zap.Logger) *AuditLogger {
	return &AuditLogger{logger: logger}
}

func (a *AuditLogger) Log(userID, action, target, detail, ip string) {
	fields := []zap.Field{
		zap.String("user_id", userID),
		zap.String("action", action),
		zap.String("ip", ip),
	}
	if target != "" {
		fields = append(fields, zap.String("target", target))
	}
	if detail != "" {
		fields = append(fields, zap.String("detail", detail))
	}
	fields = append(fields, zap.Time("event_ts", time.Now().UTC()))
	a.logger.Info("audit", fields...)
}

func (a *AuditLogger) Login(userID, ip string) {
	a.Log(userID, "login", "", "", ip)
}

func (a *AuditLogger) TaskSubmit(userID, taskID, deviceID, ip string) {
	a.Log(userID, "task_submit", taskID, deviceID, ip)
}

func (a *AuditLogger) TaskCancel(userID, taskID, ip string) {
	a.Log(userID, "task_cancel", taskID, "", ip)
}

func (a *AuditLogger) DeviceManage(userID, action, deviceID, ip string) {
	a.Log(userID, "device_"+action, deviceID, "", ip)
}

func (a *AuditLogger) AgentManage(userID, action, agentID, ip string) {
	a.Log(userID, "agent_"+action, agentID, "", ip)
}

func (a *AuditLogger) UserManage(userID, action, targetUserID, ip string) {
	a.Log(userID, "user_"+action, targetUserID, "", ip)
}

func (a *AuditLogger) ConfigChange(userID, detail, ip string) {
	a.Log(userID, "config_change", "", detail, ip)
}

func (a *AuditLogger) CredentialGen(userID, agentID, ip string) {
	a.Log(userID, "credential_gen", agentID, "", ip)
}

func (a *AuditLogger) UpgradeDispatch(userID, agentID, ip string) {
	a.Log(userID, "upgrade_dispatch", agentID, "", ip)
}

func (a *AuditLogger) AccessDenied(userID, action, target, ip string) {
	a.Log(userID, "access_denied", target, action, ip)
}