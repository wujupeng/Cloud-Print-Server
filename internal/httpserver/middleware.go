package httpserver

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(status int) {
	if w.wrote {
		return
	}
	w.status = status
	w.wrote = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.wrote = true
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("ResponseWriter does not implement http.Hijacker")
	}
	return h.Hijack()
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func RequestLogger(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(ww, r)
			logger.Info("http request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", ww.status),
				zap.Duration("latency", time.Since(start)),
				zap.String("remote", r.RemoteAddr),
			)
		})
	}
}

func Recoverer(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered",
						zap.Any("panic", rec),
						zap.ByteString("stack", debug.Stack()),
						zap.String("path", r.URL.Path),
						zap.String("method", r.Method),
					)
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"code":"INTERNAL_ERROR","message":"internal server error"}`))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowAll := len(allowedOrigins) == 0
	allowSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowSet[o] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (allowAll || containsOrigin(allowSet, origin)) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Trace-Id")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Max-Age", "3600")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func containsOrigin(set map[string]struct{}, origin string) bool {
	if _, ok := set[origin]; ok {
		return true
	}
	for allowed := range set {
		if strings.HasPrefix(allowed, "*.") {
			suffix := strings.TrimPrefix(allowed, "*")
			if strings.HasSuffix(origin, suffix) {
				return true
			}
		}
	}
	return false
}

func RateLimit(rps int, burst int) func(http.Handler) http.Handler {
	if rps <= 0 {
		rps = 100
	}
	if burst <= 0 {
		burst = rps
	}
	limiter := rate.NewLimiter(rate.Limit(rps), burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow() {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"code":"RATE_LIMIT","message":"too many requests"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}