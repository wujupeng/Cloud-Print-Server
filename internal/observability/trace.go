package observability

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type ctxKey struct{ name string }

var traceIDKey = ctxKey{"trace_id"}
var loggerKey = ctxKey{"logger"}

func WithTraceID(ctx context.Context, traceID string) context.Context {
	if traceID == "" {
		traceID = uuid.NewString()
	}
	return context.WithValue(ctx, traceIDKey, traceID)
}

func TraceIDFromCtx(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, ok := ctx.Value(traceIDKey).(string)
	if !ok || v == "" {
		return uuid.NewString()
	}
	return v
}

func NewTraceID() string {
	return uuid.NewString()
}

func WithLogger(ctx context.Context, logger *zap.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

func LoggerFromCtx(ctx context.Context, base *zap.Logger) *zap.Logger {
	if base == nil {
		base = zap.NewNop()
	}
	traceID := ""
	if ctx != nil {
		if v, ok := ctx.Value(traceIDKey).(string); ok {
			traceID = v
		}
	}
	if traceID == "" {
		return base
	}
	return base.With(zap.String("trace_id", traceID))
}