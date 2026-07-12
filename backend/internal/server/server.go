// Package server wires configuration, datastores and handlers into a running
// HTTP server with graceful shutdown (BACKEND.md §3).
package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/uptrace/bun"
	"go.uber.org/zap"

	"github.com/nazxf/opencloud/backend/internal/config"
	"github.com/nazxf/opencloud/backend/internal/handler"
	"github.com/nazxf/opencloud/backend/internal/metrics"
	"github.com/nazxf/opencloud/backend/internal/middleware"
)

// Server owns the HTTP server and its dependencies.
type Server struct {
	cfg  *config.Config
	log  *zap.Logger
	http *http.Server
}

// New builds the router, mounts middleware and routes, and returns a Server
// ready to Run. Middleware order: recovery → request-id → logger
// (auth/cors/ratelimit land in later phases — BACKEND.md §5).
func New(cfg *config.Config, log *zap.Logger, db *bun.DB, rdb *redis.Client, m *metrics.Metrics) *Server {
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(
		middleware.Recovery(log),
		middleware.RequestID(),
		middleware.Logger(log, m),
	)

	// Operational endpoints (unversioned, no auth).
	health := handler.NewHealth(db, rdb)
	r.GET("/healthz", health.Live)
	r.GET("/readyz", health.Ready)
	r.GET("/metrics", gin.WrapH(promhttp.HandlerFor(m.Registry(), promhttp.HandlerOpts{})))

	// Versioned API surface — populated as domains land.
	_ = r.Group("/api/v1")

	return &Server{
		cfg: cfg,
		log: log,
		http: &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           r,
			ReadHeaderTimeout: 10 * time.Second,
		},
	}
}

// Run starts serving and blocks until the context is cancelled (SIGTERM/SIGINT),
// then drains in-flight requests within a timeout before returning.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.log.Info("api listening", zap.String("addr", s.cfg.HTTPAddr))
		if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.log.Info("shutting down api")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return s.http.Shutdown(shutdownCtx)
	}
}
