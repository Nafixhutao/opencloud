// Package server wires configuration, datastores and handlers into a running
// HTTP server with graceful shutdown (BACKEND.md §3).
package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/uptrace/bun"
	"go.uber.org/zap"

	"github.com/nazxf/opencloud/backend/internal/config"
	"github.com/nazxf/opencloud/backend/internal/credential"
	"github.com/nazxf/opencloud/backend/internal/domainverify"
	"github.com/nazxf/opencloud/backend/internal/handler"
	logstore "github.com/nazxf/opencloud/backend/internal/logs"
	"github.com/nazxf/opencloud/backend/internal/metrics"
	"github.com/nazxf/opencloud/backend/internal/middleware"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/provisioner"
	"github.com/nazxf/opencloud/backend/internal/repository"
	"github.com/nazxf/opencloud/backend/internal/service"
)

const apiRateLimitWindow = time.Second
const minimumEdgeRateLimitRPS = 100

// Server owns the HTTP server and its dependencies.
type Server struct {
	cfg     *config.Config
	log     *zap.Logger
	http    *http.Server
	metrics *http.Server
}

// New builds the router, mounts middleware and routes, and returns a Server
// ready to Run. Middleware order: request-id → logger → recovery → cors →
// ratelimit → auth (BACKEND.md §5).
func New(
	cfg *config.Config,
	log *zap.Logger,
	db *bun.DB,
	rdb *redis.Client,
	m *metrics.Metrics,
	databaseCipher *credential.Cipher,
) *Server {
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(
		middleware.RequestID(),
		middleware.Logger(log, m),
		middleware.Recovery(log),
		cors(cfg.CORSOrigins),
	)

	// Operational endpoints (unversioned, no auth).
	health := handler.NewHealth(db, rdb, log)
	r.GET("/healthz", health.Live)
	r.GET("/readyz", health.Ready)

	// Domain services.
	acctRepo := repository.NewAccountRepo(db)
	auditRepo := repository.NewAuditRepo(db)
	siteRepo := repository.NewSiteRepo(db)
	databaseRepo := repository.NewManagedDatabaseRepo(db)
	overviewRepo := repository.NewResourceOverviewRepo(db)
	nodeRepo := repository.NewNodeRepo(db)
	jobRepo := repository.NewJobRepo(db)
	domainRepo := repository.NewDomainRepo(db)
	projectRepo := repository.NewProjectRepo(db)
	envRepo := repository.NewEnvironmentVariableRepository(db)
	acctSvc := service.NewAccountService(db, acctRepo, auditRepo)
	siteSvc := service.NewSiteService(
		db,
		siteRepo,
		nodeRepo,
		jobRepo,
		auditRepo,
		string(cfg.Provisioner.Backend),
		cfg.Provisioner.SiteImage,
		cfg.Provisioner.SiteDomainSuffix,
	)
	nodeSvc := service.NewNodeService(db, nodeRepo, auditRepo)
	databaseSvc := service.NewManagedDatabaseService(
		db,
		databaseRepo,
		jobRepo,
		auditRepo,
		cfg.CustomerDatabases.Enabled,
		databaseCipher,
	)
	overviewSvc := service.NewResourceOverviewService(overviewRepo)
	var domainSigner *domainverify.Signer
	var domainDNS provisioner.DomainDNSProvisioner
	if cfg.Domains.Enabled {
		var err error
		domainSigner, err = domainverify.New(cfg.Domains.VerificationKey)
		if err != nil {
			log.Fatal("initialize domain verification signer", zap.Error(err))
		}
		domainDNS, err = provisioner.NewManualDNS(cfg.Domains.DNSResolver)
		if err != nil {
			log.Fatal("initialize domain DNS resolver", zap.Error(err))
		}
	}
	domainSvc := service.NewDomainService(
		db, domainRepo, siteRepo, jobRepo, auditRepo,
		domainDNS, domainSigner, cfg.Domains.IngressIPv4, cfg.Domains.Enabled,
	)
	domainPermissionSvc := service.NewDomainPermissionService(
		domainRepo,
		cfg.Provisioner.SiteDomainSuffix,
	)
	projectSvc := service.NewProjectService(db, projectRepo, auditRepo)
	var customerLogs logstore.Store = logstore.UnavailableStore{}
	if cfg.Logs.Enabled {
		var err error
		customerLogs, err = logstore.NewLokiStore(
			cfg.Logs.LokiURL,
			&http.Client{Timeout: time.Duration(cfg.Logs.QueryTimeoutSeconds) * time.Second},
			time.Duration(cfg.Logs.PollIntervalSeconds)*time.Second,
		)
		if err != nil {
			log.Fatal("initialize customer log store", zap.Error(err))
		}
	}
	logSvc := service.NewLogService(projectRepo, customerLogs)
	envSvc := service.NewEnvironmentVariableService(log, envRepo, projectRepo, databaseCipher)
	acctH := handler.NewAccountHandler(acctSvc)
	siteH := handler.NewSiteHandler(siteSvc)
	nodeH := handler.NewNodeHandler(nodeSvc)
	databaseH := handler.NewManagedDatabaseHandler(databaseSvc)
	overviewH := handler.NewResourceOverviewHandler(overviewSvc)
	domainH := handler.NewDomainHandler(domainSvc)
	projectH := handler.NewProjectHandler(projectSvc)
	logH := handler.NewLogHandler(logSvc)
	envH := handler.NewEnvironmentVariableHandler(log, envSvc)
	consoleSessionSvc := service.NewDatabaseConsoleSessionService(
		repository.NewDatabaseConsoleSessionRepository(db),
		databaseRepo,
		cfg.CustomerDatabases.Enabled,
	)
	consoleQuerySvc := service.NewConsoleQueryService(
		repository.NewDatabaseConsoleSessionRepository(db),
		repository.NewConsoleQueryAuditRepository(db),
		databaseRepo,
		databaseCipher,
		cfg.CustomerDatabases.Enabled,
		log,
	)
	consoleSessionH := handler.NewDatabaseConsoleSessionHandler(consoleSessionSvc)
	consoleQueryH := handler.NewConsoleQueryHandler(consoleQuerySvc)

	v1 := r.Group("/api/v1")
	// The public edge guard limits one source IP at a deliberately coarse
	// budget. The customer budget is applied after auth and keyed by account,
	// because BFF requests from different tenants share the Next.js source IP.
	edgeLimit := cfg.RateLimitRPS * 10
	if edgeLimit < minimumEdgeRateLimitRPS {
		edgeLimit = minimumEdgeRateLimitRPS
	}
	v1.Use(middleware.RateLimit(rdb, "api-edge", edgeLimit, apiRateLimitWindow))

	// Protected customer routes require a validated JWT with account_id + role.
	if cfg.AuthJWKSURL != "" {
		keyFunc, err := waitForJWKS(cfg.AuthJWKSURL, log)
		if err != nil {
			log.Fatal("jwks init failed", zap.Error(err), zap.String("url", cfg.AuthJWKSURL))
		}
		authed := v1.Group("")
		authed.Use(
			middleware.Auth(keyFunc, cfg.AuthIssuer, cfg.AuthAudience),
			middleware.RequireCurrentMembership(acctRepo),
			middleware.RateLimit(rdb, "api-account", cfg.RateLimitRPS, apiRateLimitWindow),
		)
		{
			authed.GET("/me", acctH.Me)
			authed.PATCH("/me", middleware.RateLimit(rdb, "me-write", 30, time.Minute), acctH.UpdateMe)
			authed.GET("/overview", overviewH.Get)
			authed.GET("/projects", projectH.ListProjects)
			authed.POST("/projects", middleware.RateLimit(rdb, "project-write", 30, time.Minute), projectH.CreateProject)
			authed.GET("/projects/:projectID", projectH.GetProject)
			authed.GET("/projects/:projectID/services", projectH.ListServices)
			authed.POST("/projects/:projectID/services", middleware.RateLimit(rdb, "project-write", 30, time.Minute), projectH.CreateService)
			authed.GET("/projects/:projectID/services/:serviceID", projectH.GetService)
			authed.GET("/projects/:projectID/services/:serviceID/deployments", projectH.ListDeployments)
			authed.GET("/projects/:projectID/services/:serviceID/deployments/:deploymentID", projectH.GetDeployment)
			authed.GET("/projects/:projectID/services/:serviceID/deployments/:deploymentID/events", projectH.ListDeploymentEvents)
			authed.GET("/projects/:projectID/logs", logH.List)
			authed.GET("/projects/:projectID/logs/stream", logH.Stream)
			authed.GET("/projects/:projectId/services/:serviceId/environment", envH.List)
			authed.POST("/projects/:projectId/services/:serviceId/environment", middleware.RateLimit(rdb, "env-write", 30, time.Minute), envH.Create)
			authed.PUT("/projects/:projectId/services/:serviceId/environment/:id", middleware.RateLimit(rdb, "env-write", 30, time.Minute), envH.Update)
			authed.DELETE("/projects/:projectId/services/:serviceId/environment/:id", middleware.RateLimit(rdb, "env-write", 30, time.Minute), envH.Delete)
			authed.POST("/projects/:projectId/services/:serviceId/environment/:id/reveal", middleware.RateLimit(rdb, "env-reveal", 10, time.Minute), envH.Reveal)
			authed.GET("/projects/:projectId/services/:serviceId/environment/audit", envH.ListAudit)
			authed.GET("/sites", siteH.List)
			authed.POST("/sites", middleware.RateLimit(rdb, "site-write", 30, time.Minute), siteH.Create)
			authed.GET("/sites/:id", siteH.Get)
			authed.POST("/sites/:id/suspend", middleware.RateLimit(rdb, "site-write", 30, time.Minute), siteH.Suspend)
			authed.POST("/sites/:id/resume", middleware.RateLimit(rdb, "site-write", 30, time.Minute), siteH.Resume)
			authed.DELETE("/sites/:id", middleware.RateLimit(rdb, "site-write", 30, time.Minute), siteH.Delete)
			authed.GET("/sites/:id/domains", domainH.ListBySite)
			authed.POST("/sites/:id/domains", middleware.RateLimit(rdb, "domain-write", 30, time.Minute), domainH.Attach)
			authed.GET("/domains/:id", domainH.Get)
			authed.GET("/domains/:id/instructions", domainH.Instructions)
			authed.POST("/domains/:id/challenge", middleware.RateLimit(rdb, "domain-write", 10, time.Minute), domainH.RotateChallenge)
			authed.POST("/domains/:id/verify", middleware.RateLimit(rdb, "domain-write", 30, time.Minute), domainH.Verify)
			authed.DELETE("/domains/:id", middleware.RateLimit(rdb, "domain-write", 30, time.Minute), domainH.Detach)
			authed.GET("/databases", databaseH.List)
			authed.POST(
				"/databases",
				middleware.RateLimit(rdb, "database-write", 30, time.Minute),
				databaseH.Create,
			)
			authed.GET("/databases/:id", databaseH.Get)
			authed.DELETE(
				"/databases/:id",
				middleware.RateLimit(rdb, "database-write", 30, time.Minute),
				databaseH.Delete,
			)
			authed.POST(
				"/databases/:id/credentials/reveal",
				middleware.RateLimit(rdb, "database-credential", 10, time.Minute),
				databaseH.RevealCredential,
			)

			// Database console routes
			console := authed.Group("/databases/:databaseId/console")
			{
				console.POST("/sessions", middleware.RateLimit(rdb, "console-session", 10, time.Minute), consoleSessionH.CreateSession)
				console.DELETE("/sessions/:sessionId", middleware.RateLimit(rdb, "console-session", 30, time.Minute), consoleSessionH.RevokeSession)
				console.POST("/execute", middleware.RateLimit(rdb, "console-execute", 60, time.Second), consoleQueryH.ExecuteQuery)
			}

			admin := authed.Group("/admin")
			admin.Use(middleware.RequireRole(model.RoleAdmin))
			{
				admin.GET("/users", acctH.ListUsers)
				admin.GET("/users/:id", acctH.GetUser)
				admin.PATCH("/users/:id", middleware.RateLimit(rdb, "admin-write", 60, time.Minute), acctH.UpdateUser)
				admin.GET("/nodes", nodeH.List)
				admin.POST("/nodes", middleware.RateLimit(rdb, "admin-write", 60, time.Minute), nodeH.Register)
				admin.PATCH("/nodes/:id", middleware.RateLimit(rdb, "admin-write", 60, time.Minute), nodeH.SetStatus)
			}
		}
	} else {
		log.Warn("AUTH_JWKS_URL unset; protected /api/v1 routes are not mounted")
	}

	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.HandlerFor(m.Registry(), promhttp.HandlerOpts{}))
	metricsMux.Handle("/caddy/permission", handler.NewCaddyPermissionHandler(domainPermissionSvc))

	return &Server{
		cfg: cfg,
		log: log,
		http: &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           r,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      apiWriteTimeout(cfg.Logs.Enabled),
			IdleTimeout:       60 * time.Second,
			MaxHeaderBytes:    1 << 20,
		},
		metrics: &http.Server{
			Addr:              cfg.MetricsAddr,
			Handler:           metricsMux,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
			MaxHeaderBytes:    1 << 20,
		},
	}
}

func apiWriteTimeout(logsEnabled bool) time.Duration {
	if logsEnabled {
		// SSE responses are long-lived and carry their own 15-second heartbeat.
		// Reverse proxies must still apply idle/header limits at the public edge.
		return 0
	}
	return 30 * time.Second
}

// Run starts serving and blocks until the context is cancelled (SIGTERM/SIGINT),
// then drains in-flight requests within a timeout before returning.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 2)
	go func() {
		s.log.Info("api listening", zap.String("addr", s.cfg.HTTPAddr))
		if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	go func() {
		s.log.Info("metrics listening", zap.String("addr", s.cfg.MetricsAddr))
		if err := s.metrics.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	var runErr error
	select {
	case err := <-errCh:
		runErr = err
	case <-ctx.Done():
	}

	s.log.Info("shutting down api")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return errors.Join(runErr, s.http.Shutdown(shutdownCtx), s.metrics.Shutdown(shutdownCtx))
}

func cors(origins string) gin.HandlerFunc {
	allowed := map[string]struct{}{}
	for _, o := range strings.Split(origins, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			allowed[o] = struct{}{}
		}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			if _, ok := allowed[origin]; ok || origins == "*" {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Vary", "Origin")
				c.Header("Access-Control-Allow-Credentials", "true")
				c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-Request-ID, X-User-Name")
				c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
			}
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// waitForJWKS retries JWKS fetch so the API can boot before the BFF is ready
// (Compose starts api and dashboard in parallel). The successful Keyfunc is
// bound to a non-cancelled background context so background refresh continues.
func waitForJWKS(url string, log *zap.Logger) (jwt.Keyfunc, error) {
	var last error
	for attempt := 1; attempt <= 30; attempt++ {
		// Probe reachability first with a short timeout; only then start the
		// long-lived JWKS refresh loop on a background context.
		probeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, url, nil)
		if err == nil {
			resp, perr := http.DefaultClient.Do(req)
			if perr == nil {
				_ = resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 500 {
					cancel()
					kf, jerr := middleware.NewJWKS(context.Background(), url)
					if jerr == nil {
						if attempt > 1 {
							log.Info("jwks ready", zap.String("url", url), zap.Int("attempt", attempt))
						}
						return kf, nil
					}
					last = jerr
				} else {
					last = errors.New("jwks probe status " + resp.Status)
				}
			} else {
				last = perr
			}
		} else {
			last = err
		}
		cancel()
		log.Warn("jwks not ready; retrying",
			zap.String("url", url),
			zap.Int("attempt", attempt),
			zap.Error(last),
		)
		time.Sleep(2 * time.Second)
	}
	return nil, last
}
