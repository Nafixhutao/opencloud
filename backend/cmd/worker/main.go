// Command worker runs the OpenCloud background job worker. It shares startup
// wiring with the API, claims PostgreSQL jobs, and alone reaches the hosting
// backend (BACKEND.md section 3, ADR 0002).
package main

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/nazxf/opencloud/backend/internal/app"
	"github.com/nazxf/opencloud/backend/internal/provisioner"
	"github.com/nazxf/opencloud/backend/internal/queue"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	deps, err := app.Bootstrap(ctx, true)
	if err != nil {
		panic(err)
	}
	defer deps.Close()
	if err := deps.Cfg.ValidateProvisioner(); err != nil {
		deps.Log.Fatal("invalid provisioner configuration", zap.Error(err))
	}
	siteProvisioner, err := buildProvisioner(deps)
	if err != nil {
		deps.Log.Fatal("initialize provisioner", zap.Error(err))
	}

	sites := repository.NewSiteRepo(deps.DB)
	nodes := repository.NewNodeRepo(deps.DB)
	jobs := repository.NewJobRepo(deps.DB)
	audit := repository.NewAuditRepo(deps.DB)
	processor := queue.NewProcessor(deps.DB, sites, nodes, jobs, audit, siteProvisioner)
	runner := queue.NewRunner(deps.DB, jobs, processor, deps.Log)

	deps.Log.Info(
		"worker started",
		zap.String("provisioner_backend", string(deps.Cfg.Provisioner.Backend)),
	)
	runner.Run(ctx)
	deps.Log.Info("worker shutting down")
}

func buildProvisioner(deps *app.Deps) (provisioner.SiteProvisioner, error) {
	switch deps.Cfg.Provisioner.Backend {
	case provisioner.BackendFake:
		return provisioner.NewFake(), nil
	case provisioner.BackendDocker:
		return provisioner.NewDocker(
			deps.Cfg.Provisioner.DockerSocket,
			deps.Cfg.Provisioner.CaddyAPIURL,
			deps.Cfg.Provisioner.CaddyServerID,
			deps.Cfg.Provisioner.SiteImage,
		)
	case provisioner.BackendHestia:
		return nil, fmt.Errorf("hestia adapter is not implemented; use Docker or fake")
	default:
		return nil, fmt.Errorf("unsupported provisioner backend %q", deps.Cfg.Provisioner.Backend)
	}
}
