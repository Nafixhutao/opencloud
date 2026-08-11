package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"go.uber.org/zap"

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
	workDir      string
	domainSuffix string
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
	domainSuffix string,
) *BuildJobHandlers {
	return &BuildJobHandlers{
		log: log, db: db,
		serviceRepo: serviceRepo, previewRepo: previewRepo,
		jobRepo: jobRepo, siteRepo: siteRepo,
		git: git, siteProv: siteProv,
		workDir:      os.TempDir(),
		domainSuffix: domainSuffix,
	}
}

// Handle dispatches one build job.
func (h *BuildJobHandlers) Handle(ctx context.Context, job *model.Job, workerID string) error {
	switch job.Kind {
	case model.JobCloneGitSource:
		return h.handleClone(ctx, job, workerID)
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

	h.log.Info("cloning git source",
		zap.String("service_id", svc.ID.String()),
		zap.String("repo_url", svc.GitRepoURL),
		zap.String("branch", svc.GitBranch),
	)

	targetDir := fmt.Sprintf("%s/opencloud-clone-%s", h.workDir, svc.ID.String())
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

// handleDeployPreview provisions a temporary site for PR review.
func (h *BuildJobHandlers) handleDeployPreview(ctx context.Context, job *model.Job, workerID string) error {
	h.log.Info("deploying preview", zap.String("job_id", job.ID.String()))

	// Future: resolve service, get artifact, provision site with temp domain.
	_ = preview.GenerateDomain(job.ID.String(), h.domainSuffix)

	return h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return h.jobRepo.WithDB(tx).Complete(ctx, job.ID, workerID)
	})
}

// handleDestroyPreview tears down a temporary preview site.
func (h *BuildJobHandlers) handleDestroyPreview(ctx context.Context, job *model.Job, workerID string) error {
	h.log.Info("destroying preview", zap.String("job_id", job.ID.String()))

	// Future: delete site, remove DNS, cleanup resources.
	return h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return h.jobRepo.WithDB(tx).Complete(ctx, job.ID, workerID)
	})
}
