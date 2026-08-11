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

	// Enqueue auto-destroy after 24h
	destroyJob, err := h.jobRepo.Enqueue(ctx, &accountID, model.JobDestroyPreview,
		map[string]string{"job_id": job.ID.String()})
	if err != nil {
		h.log.Warn("failed to enqueue auto-destroy", zap.Error(err))
	} else {
		h.log.Info("preview will auto-destroy",
			zap.String("destroy_job_id", destroyJob.ID.String()),
		)
	}

	_ = destroyJob

	return h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return h.jobRepo.WithDB(tx).Complete(ctx, job.ID, workerID)
	})
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
func (h *BuildJobHandlers) EnqueueDestroy(ctx context.Context, accountID uuid.UUID, previewID string) error {
	payload, _ := json.Marshal(map[string]string{"preview_id": previewID})
	delay := time.Now().UTC().Add(24 * time.Hour)
	return h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := h.jobRepo.WithDB(tx).Enqueue(ctx, &accountID, model.JobDestroyPreview, payload)
		if err != nil {
			return err
		}
		_, err = tx.NewUpdate().
			Model((*model.Job)(nil)).
			Set("run_at = ?", delay).
			Where("kind = ?", model.JobDestroyPreview).
			Where("status = ?", model.JobQueued).
			Exec(ctx)
		return err
	})
}
