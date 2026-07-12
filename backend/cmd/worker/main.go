// Command worker runs the OpenCloud background job worker. It shares all
// wiring with cmd/api and differs only in what it starts: instead of serving
// HTTP it polls the Postgres `jobs` queue (BACKEND.md §3, ADR 0002).
//
// The queue itself lands in Phase 2; for now this is the runnable skeleton —
// it boots the same dependencies and idles on a tick until SIGTERM, so
// `docker compose up` brings a healthy worker container up alongside the API.
package main

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/nazxf/opencloud/backend/internal/app"
)

// pollInterval is how often the worker will claim jobs once the queue exists.
const pollInterval = 2 * time.Second

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	deps, err := app.Bootstrap(ctx, true)
	if err != nil {
		panic(err)
	}
	defer deps.Close()

	deps.Log.Info("worker started", zap.Duration("poll_interval", pollInterval))

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			deps.Log.Info("worker shutting down")
			return
		case <-ticker.C:
			// ponytail: no-op until the jobs queue lands (Phase 2). This is
			// where the worker will claim + run jobs with SKIP LOCKED.
		}
	}
}
