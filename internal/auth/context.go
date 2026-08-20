package auth

import (
	"context"
)

func UserIDFromCtx(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, ok := ctx.Value(userIDKey).(string)
	if !ok {
		return ""
	}
	return v
}

func RoleFromCtx(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	v, ok := ctx.Value(roleKey).(string)
	if !ok {
		return "", false
	}
	return v, true
}

func IsAdminFromCtx(ctx context.Context) bool {
	role, ok := RoleFromCtx(ctx)
	return ok && role == "ADMIN"
}

func WithUser(ctx context.Context, userID, role string) context.Context {
	ctx = context.WithValue(ctx, userIDKey, userID)
	ctx = context.WithValue(ctx, roleKey, role)
	return ctx
}