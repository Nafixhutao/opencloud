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
	// Accepts: "fake" for development/testing, "s3" for S3-compatible backends.
	// Defaults to disabled (no storage workers). Fails closed on unknown values.
	storageProviderType := os.Getenv("STORAGE_PROVIDER")
	var storageProvider provisioner.ObjectStorageProvider
	switch storageProviderType {
	case "fake":
		if deps.Cfg.IsProduction() {
			deps.Log.Fatal("STORAGE_PROVIDER=fake is not allowed in production")
		}
		deps.Log.Warn("storage worker configured with FAKE provider; FOR DEVELOPMENT/TESTING ONLY")
		storageProvider = provisioner.NewFakeStorageProvider()
	case "s3":
		s3cfg := provisioner.S3StorageConfig{
			Endpoint:        os.Getenv("STORAGE_S3_ENDPOINT"),
			Region:          os.Getenv("STORAGE_S3_REGION"),
			AccessKeyID:     os.Getenv("STORAGE_S3_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("STORAGE_S3_SECRET_ACCESS_KEY"),
			UsePathStyle:    os.Getenv("STORAGE_S3_USE_PATH_STYLE") != "false",
		}
		if s3cfg.Region == "" {
			s3cfg.Region = "us-east-1"
		}
		var err error
		storageProvider, err = provisioner.NewS3StorageProvider(ctx, s3cfg)
		if err != nil {
			deps.Log.Fatal("initialize S3 storage provider", zap.Error(err))
		}
		deps.Log.Info("storage worker configured with S3 provider",
			zap.String("endpoint", s3cfg.Endpoint),
			zap.String("region", s3cfg.Region),
			zap.Bool("use_path_style", s3cfg.UsePathStyle),
		)
	case "":
		deps.Log.Info("storage worker disabled; set STORAGE_PROVIDER=fake or STORAGE_PROVIDER=s3 to enable")
	default:
		deps.Log.Fatal("unsupported STORAGE_PROVIDER value; use 'fake', 's3', or leave unset", zap.String("value", storageProviderType))
	}

	if storageProvider != nil {
		storageHandlers := queue.NewStorageJobHandlers(
			deps.Log,
			deps.DB,
			buckets,
			jobs,
			audit,
			storageProvider,
		)
		processor.SetStorageHandlers(storageHandlers)
	}

	// Build pipeline handlers (git clone + preview deploy).
	services := repository.NewServiceRepo(deps.DB)
	previews := repository.NewPreviewDeploymentRepo(deps.DB)
	gitProvisioner := provisioner.NewLocalGitProvisioner()
	buildHandlers := queue.NewBuildJobHandlers(
		deps.Log, deps.DB,
		services, previews, jobs, sites,
		gitProvisioner, siteProvisioner,
		deps.Cfg.Provisioner.SiteDomainSuffix,
	)
	processor.SetBuildHandlers(buildHandlers)

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
