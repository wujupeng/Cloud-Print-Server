package taskmanager

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/cloud-print/server/internal/domain"
	"github.com/cloud-print/server/internal/docstore"
	"github.com/cloud-print/server/internal/errs"
	"github.com/cloud-print/server/internal/observability"
	"github.com/cloud-print/server/internal/storage"
)

type Dispatcher interface {
	SendTask(agentID string, task *domain.PrintTask) error
	SendControl(agentID string, control domain.ControlPayload) error
}

type StatusNotifier interface {
	NotifyTaskStatus(userID, taskID string, status domain.TaskStatus, payload interface{})
}

type DeviceLookup interface {
	GetDevice(ctx context.Context, deviceID string) (*domain.Device, error)
}

type TaskManager struct {
	taskRepo   *storage.TaskRepo
	docStore   *docstore.Store
	hub        Dispatcher
	notifier   StatusNotifier
	deviceRepo *storage.DeviceRepo
	logger     *zap.Logger
	audit      *observability.AuditLogger
}

func NewTaskManager(
	taskRepo *storage.TaskRepo,
	docStore *docstore.Store,
	hub Dispatcher,
	logger *zap.Logger,
	audit *observability.AuditLogger,
) *TaskManager {
	return &TaskManager{
		taskRepo: taskRepo,
		docStore: docStore,
		hub:      hub,
		logger:   logger,
		audit:    audit,
	}
}

func (tm *TaskManager) SetHub(hub Dispatcher)                 { tm.hub = hub }
func (tm *TaskManager) SetStatusNotifier(n StatusNotifier)    { tm.notifier = n }
func (tm *TaskManager) SetDeviceRepo(repo *storage.DeviceRepo) { tm.deviceRepo = repo }

func (tm *TaskManager) CreateTask(
	ctx context.Context,
	userID, deviceID, docID string,
	params domain.PrintParams,
) (*domain.PrintTask, error) {
	if userID == "" || deviceID == "" || docID == "" {
		return nil, errs.New(errs.ErrParamInvalid, "user_id, device_id, doc_id are required")
	}

	task := &domain.PrintTask{
		TaskID:      uuid.NewString(),
		UserID:      userID,
		DeviceID:    deviceID,
		DocID:       docID,
		Params:      params,
		Status:      domain.TaskStatusPending,
		TraceID:     observability.TraceIDFromCtx(ctx),
		SubmittedAt: time.Now().UTC(),
	}

	if tm.deviceRepo != nil {
		if dev, err := tm.deviceRepo.GetByID(ctx, deviceID); err == nil && dev != nil {
			task.AgentID = dev.AgentID
		}
	}

	if tm.docStore != nil {
		if checksum, err := tm.docStore.CalcChecksum(docID); err == nil {
			task.Checksum = checksum
		} else {
			tm.logger.Warn("calc checksum failed", zap.String("doc_id", docID), zap.Error(err))
		}
		task.DocumentRef = fmt.Sprintf("/api/v1/documents/%s/download", docID)
	}

	if err := tm.taskRepo.Create(ctx, task); err != nil {
		return nil, errs.Wrap(errs.ErrInternalError, "create task", err)
	}

	if tm.audit != nil {
		tm.audit.TaskSubmit(userID, task.TaskID, deviceID, "")
	}
	tm.notifyStatus(task.UserID, task.TaskID, task.Status, task)
	return task, nil
}

func (tm *TaskManager) GetTask(ctx context.Context, taskID string) (*domain.PrintTask, error) {
	t, err := tm.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return nil, errs.Wrap(errs.ErrInternalError, "get task", err)
	}
	if t == nil {
		return nil, errs.New(errs.ErrTaskNotFound, fmt.Sprintf("task %s not found", taskID))
	}
	return t, nil
}

func (tm *TaskManager) Get(ctx context.Context, taskID string) (*domain.PrintTask, error) {
	return tm.GetTask(ctx, taskID)
}

func (tm *TaskManager) ListTasks(ctx context.Context, userID string, limit, offset int) ([]*domain.PrintTask, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	return tm.taskRepo.ListByUser(ctx, userID, limit, offset)
}

func (tm *TaskManager) ListAll(ctx context.Context, limit, offset int) ([]*domain.PrintTask, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	return tm.taskRepo.ListAll(ctx, limit, offset)
}

func (tm *TaskManager) CancelTask(ctx context.Context, taskID, userID string) error {
	t, err := tm.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return errs.Wrap(errs.ErrInternalError, "get task", err)
	}
	if t == nil {
		return errs.New(errs.ErrTaskNotFound, fmt.Sprintf("task %s not found", taskID))
	}
	if t.UserID != userID {
		return errs.New(errs.ErrNoPermission, "task does not belong to user")
	}
	if t.Status != domain.TaskStatusPending && t.Status != domain.TaskStatusRunning {
		return errs.New(errs.ErrTaskNotCancellable, fmt.Sprintf("task status %s cannot be cancelled", t.Status))
	}

	if err := tm.taskRepo.UpdateStatus(ctx, taskID, domain.TaskStatusCancelled, t.RetryCount, "", ""); err != nil {
		return errs.Wrap(errs.ErrTaskCancelFail, "update status", err)
	}
	if err := tm.taskRepo.MarkFinished(ctx, taskID, domain.TaskStatusCancelled, "", ""); err != nil {
		tm.logger.Warn("mark finished failed on cancel", zap.String("task_id", taskID), zap.Error(err))
	}

	if tm.hub != nil && t.AgentID != "" {
		ctrl := domain.ControlPayload{Action: "cancel_task", TaskID: taskID}
		if err := tm.hub.SendControl(t.AgentID, ctrl); err != nil {
			tm.logger.Warn("send cancel control failed", zap.String("task_id", taskID), zap.Error(err))
		}
	}

	if tm.audit != nil {
		tm.audit.TaskCancel(userID, taskID, "")
	}
	tm.notifyStatus(t.UserID, taskID, domain.TaskStatusCancelled, nil)
	return nil
}

func (tm *TaskManager) DispatchTask(ctx context.Context, taskID string) error {
	t, err := tm.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return errs.Wrap(errs.ErrInternalError, "get task", err)
	}
	if t == nil {
		return errs.New(errs.ErrTaskNotFound, fmt.Sprintf("task %s not found", taskID))
	}
	if t.Status != domain.TaskStatusPending {
		return errs.Newf(errs.ErrTaskNotCancellable, "task status %s cannot be dispatched", t.Status)
	}
	if tm.hub == nil {
		return errs.New(errs.ErrInternalError, "dispatcher not configured")
	}
	if t.AgentID == "" {
		if tm.deviceRepo != nil {
			if dev, err := tm.deviceRepo.GetByID(ctx, t.DeviceID); err == nil && dev != nil {
				t.AgentID = dev.AgentID
			}
		}
	}
	if t.AgentID == "" {
		return errs.New(errs.ErrAgentOffline, "agent not resolved for device")
	}

	if err := tm.hub.SendTask(t.AgentID, t); err != nil {
		return errs.Wrap(errs.ErrAgentOffline, "send task", err)
	}
	if err := tm.taskRepo.MarkDispatched(ctx, taskID); err != nil {
		return errs.Wrap(errs.ErrInternalError, "mark dispatched", err)
	}
	tm.logger.Info("task dispatched",
		zap.String("task_id", taskID),
		zap.String("agent_id", t.AgentID),
		zap.String("device_id", t.DeviceID),
	)
	return nil
}

func (tm *TaskManager) HandleAck(ctx context.Context, p domain.TaskAckPayload) error {
	t, err := tm.taskRepo.GetByID(ctx, p.TaskID)
	if err != nil {
		return errs.Wrap(errs.ErrInternalError, "get task", err)
	}
	if t == nil {
		return errs.New(errs.ErrTaskNotFound, fmt.Sprintf("task %s not found", p.TaskID))
	}

	if p.Accepted {
		if err := tm.taskRepo.MarkStarted(ctx, p.TaskID); err != nil {
			return errs.Wrap(errs.ErrInternalError, "mark started", err)
		}
		tm.notifyStatus(t.UserID, p.TaskID, domain.TaskStatusRunning, p)
		tm.logger.Info("task accepted", zap.String("task_id", p.TaskID))
		return nil
	}

	if err := tm.taskRepo.UpdateStatus(ctx, p.TaskID, domain.TaskStatusFailed, 0, "TASK_REJECTED", p.Reason); err != nil {
		return errs.Wrap(errs.ErrInternalError, "update status", err)
	}
	if err := tm.taskRepo.MarkFinished(ctx, p.TaskID, domain.TaskStatusFailed, "TASK_REJECTED", p.Reason); err != nil {
		tm.logger.Warn("mark finished failed on reject", zap.String("task_id", p.TaskID), zap.Error(err))
	}
	tm.notifyStatus(t.UserID, p.TaskID, domain.TaskStatusFailed, p)
	tm.logger.Info("task rejected", zap.String("task_id", p.TaskID), zap.String("reason", p.Reason))
	return nil
}

func (tm *TaskManager) HandleResult(ctx context.Context, p domain.TaskResultPayload) error {
	t, err := tm.taskRepo.GetByID(ctx, p.TaskID)
	if err != nil {
		return errs.Wrap(errs.ErrInternalError, "get task", err)
	}
	if t == nil {
		return errs.New(errs.ErrTaskNotFound, fmt.Sprintf("task %s not found", p.TaskID))
	}

	if err := tm.taskRepo.UpdateStatus(ctx, p.TaskID, p.Status, p.RetryCount, p.ErrorCode, p.ErrorMsg); err != nil {
		return errs.Wrap(errs.ErrInternalError, "update status", err)
	}
	if p.Status.IsTerminal() {
		if err := tm.taskRepo.MarkFinished(ctx, p.TaskID, p.Status, p.ErrorCode, p.ErrorMsg); err != nil {
			return errs.Wrap(errs.ErrInternalError, "mark finished", err)
		}
	}
	tm.notifyStatus(t.UserID, p.TaskID, p.Status, p)
	tm.logger.Info("task result",
		zap.String("task_id", p.TaskID),
		zap.String("status", p.Status.String()),
		zap.Int("retry", p.RetryCount),
		zap.String("err_code", p.ErrorCode),
	)
	return nil
}

func (tm *TaskManager) MarkDispatched(ctx context.Context, taskID string) error {
	return tm.taskRepo.MarkDispatched(ctx, taskID)
}

func (tm *TaskManager) MarkStarted(ctx context.Context, taskID string) error {
	return tm.taskRepo.MarkStarted(ctx, taskID)
}

func (tm *TaskManager) MarkFinished(ctx context.Context, taskID string, status domain.TaskStatus, errCode, errMsg string) error {
	return tm.taskRepo.MarkFinished(ctx, taskID, status, errCode, errMsg)
}

func (tm *TaskManager) UpdateStatus(ctx context.Context, taskID string, status domain.TaskStatus, retryCount int, errCode, errMsg string) error {
	return tm.taskRepo.UpdateStatus(ctx, taskID, status, retryCount, errCode, errMsg)
}

func (tm *TaskManager) Submit(ctx context.Context, task *domain.PrintTask) error {
	return tm.taskRepo.Create(ctx, task)
}

func (tm *TaskManager) ListByUser(ctx context.Context, userID string, limit, offset int) ([]*domain.PrintTask, error) {
	return tm.ListTasks(ctx, userID, limit, offset)
}

func (tm *TaskManager) notifyStatus(userID, taskID string, status domain.TaskStatus, payload interface{}) {
	if tm.notifier == nil {
		return
	}
	tm.notifier.NotifyTaskStatus(userID, taskID, status, payload)
}
