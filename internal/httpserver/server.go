package httpserver

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/cloud-print/server/internal/config"
	"github.com/cloud-print/server/internal/observability"
)

type Server struct {
	cfg        *config.ServerConfig
	logger     *zap.Logger
	audit      *observability.AuditLogger
	router     chi.Router
	httpServer *http.Server
}

func NewServer(cfg *config.ServerConfig, logger *zap.Logger, audit *observability.AuditLogger) *Server {
	r := chi.NewRouter()
	s := &Server{
		cfg:    cfg,
		logger: logger,
		audit:  audit,
		router: r,
	}
	s.RegisterRoutes(r)
	s.httpServer = &http.Server{
		Addr:              cfg.Server.Listen,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return s
}

func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("http server starting", zap.String("listen", s.cfg.Server.Listen), zap.Bool("tls", s.cfg.Server.TLS.Enabled))
	go func() {
		<-ctx.Done()
		_ = s.Stop()
	}()

	var err error
	if s.cfg.Server.TLS.Enabled {
		err = s.httpServer.ListenAndServeTLS(s.cfg.Server.TLS.CertFile, s.cfg.Server.TLS.KeyFile)
	} else {
		err = s.httpServer.ListenAndServe()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) Router() chi.Router { return s.router }

func (s *Server) RegisterRoutes(r chi.Router) {
	r.Use(Recoverer(s.logger))
	r.Use(RequestLogger(s.logger))
	r.Use(CORS(s.cfg.CORS.AllowedOrigins))
	r.Use(RateLimit(100, 20))

	r.Get("/api/v1/healthz", Healthz)
	r.Get("/api/v1/status", Status(s))
}