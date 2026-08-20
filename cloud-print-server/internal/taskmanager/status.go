package taskmanager

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/cloud-print/server/internal/domain"
)

type StatusChange struct {
	TaskID    string             `json:"task_id"`
	UserID    string             `json:"user_id,omitempty"`
	Status    domain.TaskStatus  `json:"status"`
	Previous  domain.TaskStatus  `json:"previous,omitempty"`
	ChangedAt time.Time          `json:"changed_at"`
	Payload   interface{}        `json:"payload,omitempty"`
}

func (tm *TaskManager) UpdateStatusWithNotify(ctx context.Context, taskID string, status domain.TaskStatus) error {
	t, err := tm.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return err
	}
	if t == nil {
		return nil
	}
	previous := t.Status
	if err := tm.taskRepo.UpdateStatus(ctx, taskID, status, t.RetryCount, "", ""); err != nil {
		return err
	}
	tm.NotifyUser(t.UserID, taskID, status, previous, nil)
	return nil
}

func (tm *TaskManager) NotifyUser(userID, taskID string, status, previous domain.TaskStatus, payload interface{}) {
	if tm.notifier == nil {
		return
	}
	change := StatusChange{
		TaskID:    taskID,
		UserID:    userID,
		Status:    status,
		Previous:  previous,
		ChangedAt: time.Now().UTC(),
		Payload:   payload,
	}
	tm.notifier.NotifyTaskStatus(userID, taskID, status, change)
	tm.logger.Debug("task status notified",
		zap.String("task_id", taskID),
		zap.String("user_id", userID),
		zap.String("status", status.String()),
		zap.String("previous", previous.String()),
	)
}

func (tm *TaskManager) Transition(ctx context.Context, taskID string, to domain.TaskStatus) error {
	t, err := tm.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return err
	}
	if t == nil {
		return nil
	}
	if !canTransition(t.Status, to) {
		return nil
	}
	previous := t.Status
	switch to {
	case domain.TaskStatusRunning:
		if err := tm.taskRepo.MarkStarted(ctx, taskID); err != nil {
			return err
		}
	case domain.TaskStatusSuccess, domain.TaskStatusFailed, domain.TaskStatusCancelled:
		if err := tm.taskRepo.MarkFinished(ctx, taskID, to, t.ErrorCode, t.ErrorMsg); err != nil {
			return err
		}
	default:
		if err := tm.taskRepo.UpdateStatus(ctx, taskID, to, t.RetryCount, t.ErrorCode, t.ErrorMsg); err != nil {
			return err
		}
	}
	tm.NotifyUser(t.UserID, taskID, to, previous, nil)
	return nil
}

func canTransition(from, to domain.TaskStatus) bool {
	if from == to {
		return false
	}
	if from.IsTerminal() {
		return false
	}
	switch from {
	case domain.TaskStatusPending:
		return to == domain.TaskStatusRunning || to == domain.TaskStatusCancelled || to == domain.TaskStatusFailed
	case domain.TaskStatusRunning:
		return to == domain.TaskStatusSuccess || to == domain.TaskStatusFailed || to == domain.TaskStatusRetrying || to == domain.TaskStatusCancelled
	case domain.TaskStatusRetrying:
		return to == domain.TaskStatusRunning || to == domain.TaskStatusFailed || to == domain.TaskStatusCancelled
	default:
		return false
	}
}