// Command worker runs the OpenCloud background job worker. It shares startup
// wiring with the API, claims PostgreSQL jobs, and alone reaches the hosting
// backend (BACKEND.md section 3, ADR 0002).
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/nazxf/opencloud/backend/internal/app"
	"github.com/nazxf/opencloud/backend/internal/credential"
	"github.com/nazxf/opencloud/backend/internal/domainverify"
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
	var databaseProvisioner provisioner.DatabaseProvisioner
	var databaseCipher *credential.Cipher
	if deps.Cfg.CustomerDatabases.Enabled {
		databaseCipher, err = credential.New(deps.Cfg.CustomerDatabases.CredentialKey)
		if err != nil {
			deps.Log.Fatal("initialize customer database credential cipher", zap.Error(err))
		}
		sqlProvisioner, err := provisioner.NewSQLDatabaseProvisioner(
			ctx,
			deps.Cfg.CustomerDatabases.PostgresAdminURL,
			provisioner.DatabaseEndpoint{
				Host:        deps.Cfg.CustomerDatabases.PostgresHost,
				Port:        deps.Cfg.CustomerDatabases.PostgresPort,
				TLSRequired: deps.Cfg.CustomerDatabases.TLSRequired,
			},
			deps.Cfg.CustomerDatabases.MariaDBAdminDSN,
			provisioner.DatabaseEndpoint{
				Host:        deps.Cfg.CustomerDatabases.MariaDBHost,
				Port:        deps.Cfg.CustomerDatabases.MariaDBPort,
				TLSRequired: deps.Cfg.CustomerDatabases.TLSRequired,
			},
		)
		if err != nil {
			deps.Log.Fatal("initialize customer database provisioner", zap.Error(err))
		}
		defer sqlProvisioner.Close()
		databaseProvisioner = sqlProvisioner
	}

	sites := repository.NewSiteRepo(deps.DB)
	domains := repository.NewDomainRepo(deps.DB)
	databases := repository.NewManagedDatabaseRepo(deps.DB)
	buckets := repository.NewStorageBucketRepo(deps.DB)
	nodes := repository.NewNodeRepo(deps.DB)
	jobs := repository.NewJobRepo(deps.DB)
	audit := repository.NewAuditRepo(deps.DB)
	processor := queue.NewProcessor(
		deps.DB,
		sites,
		domains,
		nodes,
		jobs,
		audit,
		siteProvisioner,
		databases,
		databaseProvisioner,
		databaseCipher,
	)
	if deps.Cfg.Domains.Enabled {
		domainSigner, err := domainverify.New(deps.Cfg.Domains.VerificationKey)
		if err != nil {
			deps.Log.Fatal("initialize domain verification signer", zap.Error(err))
		}
		domainDNS, err := provisioner.NewManualDNS(deps.Cfg.Domains.DNSResolver)
		if err != nil {
			deps.Log.Fatal("initialize domain DNS resolver", zap.Error(err))
		}
		processor.SetDomainProcessor(queue.NewDomainProcessor(
			deps.DB,
			domains,
			sites,
			jobs,
			audit,
			domainDNS,
			siteProvisioner,
			domainSigner,
			deps.Cfg.Domains.IngressIPv4,
		))
	}

	// Storage provider selection via STORAGE_PROVIDER environment variable.
	// Accepts: "fake" for development/testing only. Defaults to disabled (no storage workers).
	storageProviderType := os.Getenv("STORAGE_PROVIDER")
	if storageProviderType == "fake" {
		deps.Log.Warn("storage worker configured with FAKE provider; FOR DEVELOPMENT/TESTING ONLY")
		storageProvider := provisioner.NewFakeStorageProvider()
		storageHandlers := queue.NewStorageJobHandlers(
			deps.Log,
			deps.DB,
			buckets,
			jobs,
			audit,
			storageProvider,
		)
		processor.SetStorageHandlers(storageHandlers)
	} else if storageProviderType != "" {
		deps.Log.Fatal("unsupported STORAGE_PROVIDER value; use 'fake' or unset", zap.String("value", storageProviderType))
	} else {
		deps.Log.Info("storage worker disabled; set STORAGE_PROVIDER=fake for development/testing")
	}

	runner := queue.NewRunner(deps.DB, jobs, processor, deps.Log)

	deps.Log.Info(
		"worker started",
		zap.String("provisioner_backend", string(deps.Cfg.Provisioner.Backend)),
		zap.Bool("customer_databases_enabled", deps.Cfg.CustomerDatabases.Enabled),
		zap.Bool("domains_enabled", deps.Cfg.Domains.Enabled),
		zap.String("storage_provider", storageProviderType),
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
