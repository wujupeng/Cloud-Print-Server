package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/cloud-print/server/internal/errs"
)

type ctxKey struct{ name string }

var (
	userIDKey = ctxKey{"user_id"}
	roleKey   = ctxKey{"role"}
)

func JWTMiddleware(mgr *JWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := extractBearer(r)
			if err != nil {
				writeAuthError(w, err)
				return
			}
			claims, err := mgr.ParseToken(token)
			if err != nil {
				writeAuthError(w, err)
				return
			}
			ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
			ctx = context.WithValue(ctx, roleKey, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func AdminOnlyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role, _ := RoleFromCtx(r.Context())
		if role != "ADMIN" {
			writeAuthError(w, ErrAdminRequired)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func extractBearer(r *http.Request) (string, error) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", ErrTokenMissing
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return "", ErrTokenInvalid
	}
	token := strings.TrimPrefix(h, prefix)
	if token == "" {
		return "", ErrTokenInvalid
	}
	return token, nil
}

func writeAuthError(w http.ResponseWriter, err error) {
	apiErr, ok := err.(*errs.APIError)
	if !ok {
		apiErr = errs.New(errs.ErrAuthInvalid, err.Error())
	}
	status := apiErr.HTTPStatus
	if status == 0 {
		status = http.StatusUnauthorized
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"code":"` + string(apiErr.Code) + `","message":"` + apiErr.Message + `"}`))
}