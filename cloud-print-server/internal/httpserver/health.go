package httpserver

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"github.com/cloud-print/server/internal/domain"
)

func Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func Status(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"status":     "ok",
			"version":    domain.Version,
			"listen":     s.cfg.Server.Listen,
			"domain":     s.cfg.Server.Domain,
			"tls":        s.cfg.Server.TLS.Enabled,
			"now":        time.Now().UTC().Format(time.RFC3339),
			"go_version": runtime.Version(),
			"goroutines": runtime.NumGoroutine(),
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(resp)
	}
}