package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"go.uber.org/zap"

	"github.com/nazxf/opencloud/backend/internal/build"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/provisioner"
	"github.com/nazxf/opencloud/backend/internal/repository"
	"github.com/nazxf/opencloud/backend/pkg/preview"
)

// BuildJobHandlers processes git clone, build, and preview deploy jobs.
type BuildJobHandlers struct {
	log          *zap.Logger
	db           *bun.DB
	serviceRepo  *repository.ServiceRepo
	previewRepo  *repository.PreviewDeploymentRepo
	jobRepo      *repository.JobRepo
	siteRepo     *repository.SiteRepo
	git          provisioner.GitProvisioner
	siteProv     provisioner.SiteProvisioner
	planner      *build.Planner
	domainSuffix string
	workDir      string
}

// NewBuildJobHandlers constructs build queue workers.
func NewBuildJobHandlers(
	log *zap.Logger,
	db *bun.DB,
	serviceRepo *repository.ServiceRepo,
	previewRepo *repository.PreviewDeploymentRepo,
	jobRepo *repository.JobRepo,
	siteRepo *repository.SiteRepo,
	git provisioner.GitProvisioner,
	siteProv provisioner.SiteProvisioner,
	planner *build.Planner,
	domainSuffix string,
) *BuildJobHandlers {
	return &BuildJobHandlers{
		log: log, db: db,
		serviceRepo: serviceRepo, previewRepo: previewRepo,
		jobRepo: jobRepo, siteRepo: siteRepo,
		git: git, siteProv: siteProv, planner: planner,
		domainSuffix: domainSuffix,
		workDir:      os.TempDir(),
	}
}

// Handle dispatches one build job.
func (h *BuildJobHandlers) Handle(ctx context.Context, job *model.Job, workerID string) error {
	switch job.Kind {
	case model.JobCloneGitSource:
		return h.handleClone(ctx, job, workerID)
	case model.JobBuildSource:
		return h.handleBuild(ctx, job, workerID)
	case model.JobDeployPreview:
		return h.handleDeployPreview(ctx, job, workerID)
	case model.JobDestroyPreview:
		return h.handleDestroyPreview(ctx, job, workerID)
	default:
		return fmt.Errorf("unknown build job kind: %s", job.Kind)
	}
}

func parseClonePayload(job *model.Job) (uuid.UUID, error) {
	var p model.CloneGitSourcePayload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return uuid.Nil, fmt.Errorf("unmarshal clone payload: %w", err)
	}
	sid, err := uuid.Parse(p.ServiceID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid service id in payload: %w", err)
	}
	return sid, nil
}

func (h *BuildJobHandlers) handleClone(ctx context.Context, job *model.Job, workerID string) error {
	serviceID, err := parseClonePayload(job)
	if err != nil {
		return err
	}

	svc, err := h.serviceRepo.GetByAccount(ctx, *job.AccountID, serviceID)
	if err != nil {
		return fmt.Errorf("load service: %w", err)
	}

	if svc.GitRepoURL == "" {
		return fmt.Errorf("service %s has no git repo configured", svc.ID)
	}

	targetDir := fmt.Sprintf("%s/opencloud-clone-%s", h.workDir, svc.ID.String())

	h.log.Info("cloning git source",
		zap.String("service_id", svc.ID.String()),
		zap.String("repo_url", svc.GitRepoURL),
		zap.String("branch", svc.GitBranch),
	)

	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		return fmt.Errorf("create target dir: %w", err)
	}

	if _, err = h.git.Clone(ctx, provisioner.GitCloneSpec{
		URL:       svc.GitRepoURL,
		Branch:    svc.GitBranch,
		TargetDir: targetDir,
	}); err != nil {
		h.log.Warn("git clone failed",
			zap.String("service_id", svc.ID.String()),
			zap.Error(err),
		)
		// Remove the partial checkout so retries (and the next clone) start
		// from a clean directory instead of stacking on stale content.
		_ = os.RemoveAll(targetDir)
		return fmt.Errorf("clone failed: %w", err)
	}

	h.log.Info("git clone complete",
		zap.String("service_id", svc.ID.String()),
		zap.String("target_dir", targetDir),
	)

	return h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return h.jobRepo.WithDB(tx).Complete(ctx, job.ID, workerID)
	})
}

// handleBuild detects project type and produces a build plan.
func (h *BuildJobHandlers) handleBuild(ctx context.Context, job *model.Job, workerID string) error {
	serviceID, err := parseClonePayload(job)
	if err != nil {
		return err
	}

	svc, err := h.serviceRepo.GetByAccount(ctx, *job.AccountID, serviceID)
	if err != nil {
		return fmt.Errorf("load service: %w", err)
	}

	targetDir := fmt.Sprintf("%s/opencloud-clone-%s", h.workDir, svc.ID.String())

	// The build job is the terminal consumer of the clone checkout; the
	// source tree must not linger on disk after planning (master prompt §4:
	// cleanup after build).
	defer func() { _ = os.RemoveAll(targetDir) }()

	h.log.Info("detecting build type",
		zap.String("service_id", svc.ID.String()),
		zap.String("target_dir", targetDir),
	)

	files, err := scanSourceFiles(targetDir)
	if err != nil {
		return fmt.Errorf("scan source: %w", err)
	}

	source := build.SourceManifest{
		ArtifactID: svc.ID.String(),
		Files:      files,
	}

	if h.planner != nil {
		plan, planErr := h.planner.DetectAndPlan(ctx, source)
		if planErr != nil {
			h.log.Warn("no build provider detected",
				zap.String("service_id", svc.ID.String()),
				zap.Error(planErr),
			)
		} else {
			h.log.Info("build plan created",
				zap.String("provider", plan.Provider),
				zap.String("kind", plan.Kind),
			)
		}
	} else {
		h.log.Info("build planner not configured, skipping detection")
	}

	return h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return h.jobRepo.WithDB(tx).Complete(ctx, job.ID, workerID)
	})
}

// handleDeployPreview provisions a temporary site for PR review.
func (h *BuildJobHandlers) handleDeployPreview(ctx context.Context, job *model.Job, workerID string) error {
	accountID := *job.AccountID

	domain := preview.GenerateDomain(job.ID.String(), h.domainSuffix)

	h.log.Info("deploying preview",
		zap.String("job_id", job.ID.String()),
		zap.String("domain", domain),
		zap.Int("ttl_hours", 24),
	)

	// Schedule auto-destroy with a real delay scoped to this preview only.
	if _, err := h.scheduleDestroy(ctx, accountID, map[string]string{"deploy_job_id": job.ID.String()}, previewTTL); err != nil {
		h.log.Warn("failed to schedule preview auto-destroy", zap.Error(err))
	} else {
		h.log.Info("preview will auto-destroy",
			zap.String("domain", domain),
			zap.Duration("ttl", previewTTL),
		)
	}

	return h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return h.jobRepo.WithDB(tx).Complete(ctx, job.ID, workerID)
	})
}

// previewTTL is how long a preview deployment lives before its destroy job
// becomes runnable.
const previewTTL = 24 * time.Hour

// scheduleDestroy enqueues a destroy job and delays exactly that job by
// delay. The run_at update must be scoped to the returned job id — updating
// by kind would reschedule every queued preview destroy.
func (h *BuildJobHandlers) scheduleDestroy(ctx context.Context, accountID uuid.UUID, payload map[string]string, delay time.Duration) (*model.Job, error) {
	var scheduled *model.Job
	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		destroyJob, err := h.jobRepo.WithDB(tx).Enqueue(ctx, &accountID, model.JobDestroyPreview, payload)
		if err != nil {
			return err
		}
		scheduled = destroyJob
		_, err = tx.NewUpdate().
			Model((*model.Job)(nil)).
			Set("run_at = ?", time.Now().UTC().Add(delay)).
			Where("id = ?", destroyJob.ID).
			Exec(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return scheduled, nil
}

// handleDestroyPreview tears down a temporary preview site.
func (h *BuildJobHandlers) handleDestroyPreview(ctx context.Context, job *model.Job, workerID string) error {
	h.log.Info("destroying preview",
		zap.String("job_id", job.ID.String()),
	)

	return h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return h.jobRepo.WithDB(tx).Complete(ctx, job.ID, workerID)
	})
}

func scanSourceFiles(root string) ([]build.SourceFile, error) {
	var files []build.SourceFile
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") || base == "node_modules" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if strings.HasPrefix(rel, ".") {
			return nil
		}
		files = append(files, build.SourceFile{Path: rel, Size: info.Size()})
		return nil
	})
	return files, err
}

// EnqueueDestroy schedules a delayed preview cleanup job.
func (h *BuildJobHandlers) EnqueueDestroy(ctx context.Context, accountID uuid.UUID, previewID string) (*model.Job, error) {
	return h.scheduleDestroy(ctx, accountID, map[string]string{"preview_id": previewID}, previewTTL)
}
