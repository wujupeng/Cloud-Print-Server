package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/cloud-print/server/internal/domain"
)

type TaskRepo struct {
	*Repository
}

func NewTaskRepo(db *DB) *TaskRepo {
	return &TaskRepo{Repository: NewRepository(db)}
}

func (r *TaskRepo) Create(ctx context.Context, t *domain.PrintTask) error {
	paramsJSON := marshalParams(t.Params)
	now := time.Now().UTC()
	if t.SubmittedAt.IsZero() {
		t.SubmittedAt = now
	}
	_, err := r.ExecContext(ctx, `INSERT INTO tasks
		(task_id, user_id, device_id, agent_id, doc_id, document_ref, checksum, params, status, retry_count, trace_id, submitted_at, dispatched_at, received_at, started_at, finished_at, next_retry_at, error_code, error_msg)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.TaskID, t.UserID, t.DeviceID, t.AgentID, t.DocID, t.DocumentRef, t.Checksum, paramsJSON, t.Status, t.RetryCount, t.TraceID,
		t.SubmittedAt, t.DispatchedAt, t.ReceivedAt, t.StartedAt, t.FinishedAt, t.NextRetryAt, t.ErrorCode, t.ErrorMsg)
	if err != nil {
		return fmt.Errorf("insert task: %w", err)
	}
	return nil
}

func (r *TaskRepo) GetByID(ctx context.Context, taskID string) (*domain.PrintTask, error) {
	t := &domain.PrintTask{}
	var paramsJSON sql.NullString
	var agentID, docID, docRef, checksum, traceID, errCode, errMsg sql.NullString
	var dispatched, received, started, finished, nextRetry sql.NullTime
	err := r.QueryRowContext(ctx, `SELECT task_id, user_id, device_id, agent_id, doc_id, document_ref, checksum, params, status, retry_count, trace_id, submitted_at, dispatched_at, received_at, started_at, finished_at, next_retry_at, error_code, error_msg
		FROM tasks WHERE task_id = ?`, taskID).Scan(
		&t.TaskID, &t.UserID, &t.DeviceID, &agentID, &docID, &docRef, &checksum, &paramsJSON, &t.Status, &t.RetryCount, &traceID,
		&t.SubmittedAt, &dispatched, &received, &started, &finished, &nextRetry, &errCode, &errMsg)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	t.AgentID = agentID.String
	t.DocID = docID.String
	t.DocumentRef = docRef.String
	t.Checksum = checksum.String
	t.TraceID = traceID.String
	t.ErrorCode = errCode.String
	t.ErrorMsg = errMsg.String
	if dispatched.Valid {
		t.DispatchedAt = dispatched.Time
	}
	if received.Valid {
		t.ReceivedAt = received.Time
	}
	if started.Valid {
		t.StartedAt = started.Time
	}
	if finished.Valid {
		t.FinishedAt = finished.Time
	}
	if nextRetry.Valid {
		t.NextRetryAt = nextRetry.Time
	}
	t.Params = unmarshalParams(paramsJSON.String)
	return t, nil
}

func (r *TaskRepo) UpdateStatus(ctx context.Context, taskID string, status domain.TaskStatus, retryCount int, errCode, errMsg string) error {
	_, err := r.ExecContext(ctx, `UPDATE tasks SET status=?, retry_count=?, error_code=?, error_msg=?, updated_at=CURRENT_TIMESTAMP WHERE task_id=?`,
		status, retryCount, errCode, errMsg, taskID)
	if err != nil {
		return fmt.Errorf("update task status: %w", err)
	}
	return nil
}

func (r *TaskRepo) MarkDispatched(ctx context.Context, taskID string) error {
	_, err := r.ExecContext(ctx, `UPDATE tasks SET dispatched_at=?, updated_at=CURRENT_TIMESTAMP WHERE task_id=?`,
		time.Now().UTC(), taskID)
	if err != nil {
		return fmt.Errorf("mark dispatched: %w", err)
	}
	return nil
}

func (r *TaskRepo) MarkStarted(ctx context.Context, taskID string) error {
	_, err := r.ExecContext(ctx, `UPDATE tasks SET status=?, started_at=?, updated_at=CURRENT_TIMESTAMP WHERE task_id=?`,
		domain.TaskStatusRunning, time.Now().UTC(), taskID)
	if err != nil {
		return fmt.Errorf("mark started: %w", err)
	}
	return nil
}

func (r *TaskRepo) MarkFinished(ctx context.Context, taskID string, status domain.TaskStatus, errCode, errMsg string) error {
	_, err := r.ExecContext(ctx, `UPDATE tasks SET status=?, finished_at=?, error_code=?, error_msg=?, updated_at=CURRENT_TIMESTAMP WHERE task_id=?`,
		status, time.Now().UTC(), errCode, errMsg, taskID)
	if err != nil {
		return fmt.Errorf("mark finished: %w", err)
	}
	return nil
}

func (r *TaskRepo) ListByUser(ctx context.Context, userID string, limit, offset int) ([]*domain.PrintTask, error) {
	return r.queryList(ctx, `SELECT task_id, user_id, device_id, agent_id, doc_id, document_ref, checksum, params, status, retry_count, trace_id, submitted_at, dispatched_at, received_at, started_at, finished_at, next_retry_at, error_code, error_msg
		FROM tasks WHERE user_id=? ORDER BY submitted_at DESC LIMIT ? OFFSET ?`, userID, limit, offset)
}

func (r *TaskRepo) ListByAgentAndStatus(ctx context.Context, agentID string, status domain.TaskStatus) ([]*domain.PrintTask, error) {
	return r.queryList(ctx, `SELECT task_id, user_id, device_id, agent_id, doc_id, document_ref, checksum, params, status, retry_count, trace_id, submitted_at, dispatched_at, received_at, started_at, finished_at, next_retry_at, error_code, error_msg
		FROM tasks WHERE agent_id=? AND status=? ORDER BY submitted_at ASC`, agentID, status)
}

func (r *TaskRepo) ListAll(ctx context.Context, limit, offset int) ([]*domain.PrintTask, error) {
	return r.queryList(ctx, `SELECT task_id, user_id, device_id, agent_id, doc_id, document_ref, checksum, params, status, retry_count, trace_id, submitted_at, dispatched_at, received_at, started_at, finished_at, next_retry_at, error_code, error_msg
		FROM tasks ORDER BY submitted_at DESC LIMIT ? OFFSET ?`, limit, offset)
}

func (r *TaskRepo) queryList(ctx context.Context, query string, args ...interface{}) ([]*domain.PrintTask, error) {
	rows, err := r.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.PrintTask
	for rows.Next() {
		t := &domain.PrintTask{}
		var paramsJSON sql.NullString
		var agentID, docID, docRef, checksum, traceID, errCode, errMsg sql.NullString
		var dispatched, received, started, finished, nextRetry sql.NullTime
		if err := rows.Scan(&t.TaskID, &t.UserID, &t.DeviceID, &agentID, &docID, &docRef, &checksum, &paramsJSON, &t.Status, &t.RetryCount, &traceID,
			&t.SubmittedAt, &dispatched, &received, &started, &finished, &nextRetry, &errCode, &errMsg); err != nil {
			return nil, err
		}
		t.AgentID = agentID.String
		t.DocID = docID.String
		t.DocumentRef = docRef.String
		t.Checksum = checksum.String
		t.TraceID = traceID.String
		t.ErrorCode = errCode.String
		t.ErrorMsg = errMsg.String
		if dispatched.Valid {
			t.DispatchedAt = dispatched.Time
		}
		if received.Valid {
			t.ReceivedAt = received.Time
		}
		if started.Valid {
			t.StartedAt = started.Time
		}
		if finished.Valid {
			t.FinishedAt = finished.Time
		}
		if nextRetry.Valid {
			t.NextRetryAt = nextRetry.Time
		}
		t.Params = unmarshalParams(paramsJSON.String)
		out = append(out, t)
	}
	return out, rows.Err()
}

func marshalParams(p domain.PrintParams) string {
	if p.Copies == 0 && p.Orientation == "" && len(p.Extra) == 0 {
		return ""
	}
	b, _ := jsonMarshalParams(p)
	return string(b)
}