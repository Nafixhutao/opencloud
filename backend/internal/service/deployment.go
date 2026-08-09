package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	runtimeprovider "github.com/nazxf/opencloud/backend/internal/deployment"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/registry"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

const deploymentUnlockTimeout = 5 * time.Second

var errDeploymentAlreadyActive = errors.New("deployment is already active")

// DeploymentService is called by the restricted deployment worker after the
// isolated builder has produced an OCI image. It must never be constructed in
// an HTTP handler or with a public API/Docker socket capability.
type DeploymentService struct {
	db       *bun.DB
	projects *repository.ProjectRepo
	registry registry.Provider
	runtime  runtimeprovider.RuntimeProvider
}

// NewDeploymentService wires only the narrow registry and runtime contracts
// needed by the deployment worker.
func NewDeploymentService(
	db *bun.DB,
	projects *repository.ProjectRepo,
	registryProvider registry.Provider,
	runtime runtimeprovider.RuntimeProvider,
) (*DeploymentService, error) {
	if db == nil || projects == nil {
		return nil, errors.New("deployment database and repository are required")
	}
	if registryProvider == nil {
		return nil, errors.New("deployment registry provider is required")
	}
	if runtime == nil {
		return nil, errors.New("deployment runtime provider is required")
	}
	return &DeploymentService{db: db, projects: projects, registry: registryProvider, runtime: runtime}, nil
}

// PublishDeploymentRequest is the safe handoff from the isolated builder to
// the private registry. The request has no source path, repository credential,
// command, or tag; its digest is immutable and its target is tenant-scoped.
type PublishDeploymentRequest struct {
	Repository     registry.Repository
	SourceDigest   string
	SourceBytes    int64
	BuildProvider  string
	SourceRevision *string
}

// Publish creates the next immutable deployment revision after private registry
// publication and digest resolution both succeed. A failed publish creates no
// control-plane deployment row.
func (s *DeploymentService) Publish(
	ctx context.Context,
	accountID, projectID, serviceID uuid.UUID,
	request PublishDeploymentRequest,
) (*model.Deployment, error) {
	buildProvider, sourceRevision, err := validatePublishRequest(accountID, projectID, serviceID, request)
	if err != nil {
		return nil, err
	}
	if _, err := s.projects.GetServiceByAccount(ctx, accountID, projectID, serviceID); err != nil {
		return nil, fmt.Errorf("load deployment service: %w", err)
	}

	artifact, err := s.registry.Push(ctx, registry.PushRequest{
		Repository: request.Repository, SourceDigest: request.SourceDigest, SourceBytes: request.SourceBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("publish OCI image: %w", err)
	}
	if artifact.Repository != request.Repository {
		return nil, errors.New("registry returned an artifact outside the requested service repository")
	}
	artifact, err = s.requireStoredArtifact(ctx, artifact)
	if err != nil {
		return nil, err
	}

	var created *model.Deployment
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		repo := s.projects.WithDB(tx)
		if _, err := repo.GetServiceByAccountForUpdate(ctx, accountID, projectID, serviceID); err != nil {
			return err
		}
		if err := repo.LockDeploymentService(ctx, accountID, projectID, serviceID); err != nil {
			return err
		}
		revision, err := repo.NextDeploymentRevision(ctx, accountID, projectID, serviceID)
		if err != nil {
			return err
		}
		size := artifact.SizeBytes
		deployment := &model.Deployment{
			ID: uuid.New(), AccountID: accountID, ProjectID: projectID, ServiceID: serviceID,
			Revision: revision, ImageReference: artifact.Repository.Name(), ImageDigest: artifact.Digest,
			ImageSizeBytes: &size, BuildProvider: buildProvider, SourceRevision: sourceRevision,
			Status: model.DeploymentPushing, IsActive: false,
		}
		if err := repo.CreateDeployment(ctx, deployment); err != nil {
			return err
		}
		if err := repo.AppendDeploymentEvent(
			ctx, accountID, projectID, serviceID, deployment.ID,
			"deployment.image_pushed", "Image pushed to private registry",
			map[string]any{"image_digest": artifact.Digest, "image_size_bytes": artifact.SizeBytes},
		); err != nil {
			return err
		}
		created = deployment
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("persist immutable deployment revision: %w", err)
	}
	return created, nil
}

// Deploy starts a newly published revision, verifies its health, atomically
// switches Caddy traffic, then retires the previous revision. The service-level
// session lock serializes the full external sequence across workers.
func (s *DeploymentService) Deploy(
	ctx context.Context,
	accountID, projectID, serviceID, deploymentID uuid.UUID,
) (result *model.Deployment, err error) {
	err = s.withRuntimeLock(ctx, accountID, projectID, serviceID, func(conn bun.Conn) error {
		candidate, previous, err := s.markDeploying(ctx, conn, accountID, projectID, serviceID, deploymentID)
		if errors.Is(err, errDeploymentAlreadyActive) {
			result = candidate
			return nil
		}
		if err != nil {
			return err
		}
		artifact, err := artifactFromDeployment(candidate)
		if err != nil {
			return s.failDeployment(ctx, conn, candidate, "Deployment artifact is invalid", err)
		}
		artifact, err = s.requireStoredArtifact(ctx, artifact)
		if err != nil {
			return s.failDeployment(ctx, conn, candidate, "Deployment image is unavailable", err)
		}
		target := runtimeRevision(candidate, artifact)
		if err := s.runtime.Start(ctx, target); err != nil {
			return s.failDeployment(ctx, conn, candidate, "Runtime start failed", err)
		}
		if err := s.runtime.CheckHealth(ctx, target); err != nil {
			_ = s.runtime.Retire(context.WithoutCancel(ctx), target)
			return s.failDeployment(ctx, conn, candidate, "Runtime health check failed", err)
		}

		traffic := runtimeprovider.TrafficSwitch{Target: target}
		if previous != nil {
			previousArtifact, parseErr := artifactFromDeployment(previous)
			if parseErr != nil {
				_ = s.runtime.Retire(context.WithoutCancel(ctx), target)
				return s.failDeployment(ctx, conn, candidate, "Active deployment artifact is invalid", parseErr)
			}
			previousArtifact, resolveErr := s.requireStoredArtifact(ctx, previousArtifact)
			if resolveErr != nil {
				_ = s.runtime.Retire(context.WithoutCancel(ctx), target)
				return s.failDeployment(ctx, conn, candidate, "Active deployment image is unavailable", resolveErr)
			}
			previousRuntime := runtimeRevision(previous, previousArtifact)
			traffic.Previous = &previousRuntime
		}
		if err := s.runtime.SwitchCaddyTraffic(ctx, traffic); err != nil {
			// RuntimeProvider's contract requires a returned error to leave the old
			// route intact; this lets the candidate be safely retired and failed.
			_ = s.runtime.Retire(context.WithoutCancel(ctx), target)
			return s.failDeployment(ctx, conn, candidate, "Traffic switch failed", err)
		}

		active, retired, err := s.markReady(ctx, conn, accountID, projectID, serviceID, deploymentID)
		if err != nil {
			return err
		}
		result = active
		if retired != nil {
			retiredArtifact, parseErr := artifactFromDeployment(retired)
			if parseErr != nil {
				return s.recordRetirementPending(ctx, conn, active, "Previous runtime requires operator review")
			}
			if err := s.runtime.Retire(ctx, runtimeRevision(retired, retiredArtifact)); err != nil {
				return s.recordRetirementPending(ctx, conn, active, "Previous runtime retirement pending")
			}
		}
		return nil
	})
	return result, err
}

// Rollback activates a previously healthy immutable revision. It starts and
// verifies the old image again if necessary, switches traffic only after health
// succeeds, and never rewrites that revision's image identity.
func (s *DeploymentService) Rollback(
	ctx context.Context,
	accountID, projectID, serviceID, targetID uuid.UUID,
) (result *model.Deployment, err error) {
	err = s.withRuntimeLock(ctx, accountID, projectID, serviceID, func(conn bun.Conn) error {
		target, previous, err := s.loadRollback(ctx, conn, accountID, projectID, serviceID, targetID)
		if err != nil {
			return err
		}
		if previous != nil && previous.ID == target.ID {
			result = target
			return nil
		}
		artifact, err := artifactFromDeployment(target)
		if err != nil {
			return err
		}
		artifact, err = s.requireStoredArtifact(ctx, artifact)
		if err != nil {
			return fmt.Errorf("rollback image is unavailable: %w", err)
		}
		targetRuntime := runtimeRevision(target, artifact)
		if err := s.runtime.Start(ctx, targetRuntime); err != nil {
			return fmt.Errorf("start rollback runtime: %w", err)
		}
		if err := s.runtime.CheckHealth(ctx, targetRuntime); err != nil {
			_ = s.runtime.Retire(context.WithoutCancel(ctx), targetRuntime)
			return fmt.Errorf("health check rollback runtime: %w", err)
		}
		traffic := runtimeprovider.TrafficSwitch{Target: targetRuntime}
		if previous != nil {
			previousArtifact, err := artifactFromDeployment(previous)
			if err != nil {
				return err
			}
			previousRuntime := runtimeRevision(previous, previousArtifact)
			traffic.Previous = &previousRuntime
		}
		if err := s.runtime.SwitchCaddyTraffic(ctx, traffic); err != nil {
			_ = s.runtime.Retire(context.WithoutCancel(ctx), targetRuntime)
			return fmt.Errorf("switch Caddy traffic for rollback: %w", err)
		}
		active, retired, err := s.markRollbackActive(ctx, conn, accountID, projectID, serviceID, targetID)
		if err != nil {
			return err
		}
		result = active
		if retired != nil {
			retiredArtifact, err := artifactFromDeployment(retired)
			if err != nil {
				return s.recordRetirementPending(ctx, conn, active, "Previous runtime requires operator review")
			}
			if err := s.runtime.Retire(ctx, runtimeRevision(retired, retiredArtifact)); err != nil {
				return s.recordRetirementPending(ctx, conn, active, "Previous runtime retirement pending")
			}
		}
		return nil
	})
	return result, err
}

func (s *DeploymentService) markDeploying(
	ctx context.Context,
	conn bun.Conn,
	accountID, projectID, serviceID, deploymentID uuid.UUID,
) (*model.Deployment, *model.Deployment, error) {
	var candidate, previous *model.Deployment
	err := conn.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		repo := s.projects.WithDB(tx)
		if _, err := repo.GetServiceByAccountForUpdate(ctx, accountID, projectID, serviceID); err != nil {
			return err
		}
		var err error
		candidate, err = repo.GetDeploymentByAccountForUpdate(ctx, accountID, projectID, serviceID, deploymentID)
		if err != nil {
			return err
		}
		if candidate.Status == model.DeploymentReady && candidate.IsActive {
			return errDeploymentAlreadyActive
		}
		if candidate.Status != model.DeploymentPushing && candidate.Status != model.DeploymentScanning {
			return fmt.Errorf("deployment is not ready to start")
		}
		previous, err = repo.GetActiveDeploymentForUpdate(ctx, accountID, projectID, serviceID)
		if errors.Is(err, sql.ErrNoRows) {
			previous = nil
		} else if err != nil {
			return err
		}
		if err := repo.MarkDeploymentDeploying(ctx, candidate.ID); err != nil {
			return err
		}
		if err := repo.AppendDeploymentEvent(ctx, accountID, projectID, serviceID, candidate.ID, "deployment.started", "Starting runtime deployment", nil); err != nil {
			return err
		}
		candidate.Status = model.DeploymentDeploying
		return nil
	})
	return candidate, previous, err
}

func (s *DeploymentService) markReady(
	ctx context.Context,
	conn bun.Conn,
	accountID, projectID, serviceID, deploymentID uuid.UUID,
) (*model.Deployment, *model.Deployment, error) {
	var active, previous *model.Deployment
	err := conn.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		repo := s.projects.WithDB(tx)
		if _, err := repo.GetServiceByAccountForUpdate(ctx, accountID, projectID, serviceID); err != nil {
			return err
		}
		var err error
		active, previous, err = repo.ActivateDeployment(ctx, accountID, projectID, serviceID, deploymentID)
		if err != nil {
			return err
		}
		if err := repo.AppendDeploymentEvent(ctx, accountID, projectID, serviceID, active.ID, "deployment.ready", "Deployment passed health checks", nil); err != nil {
			return err
		}
		if err := repo.AppendDeploymentEvent(ctx, accountID, projectID, serviceID, active.ID, "deployment.traffic_switched", "Traffic switched to deployment", nil); err != nil {
			return err
		}
		if previous != nil {
			if err := repo.AppendDeploymentEvent(ctx, accountID, projectID, serviceID, previous.ID, "deployment.superseded", "Traffic switched to a newer deployment", nil); err != nil {
				return err
			}
		}
		return nil
	})
	return active, previous, err
}

func (s *DeploymentService) loadRollback(
	ctx context.Context,
	conn bun.Conn,
	accountID, projectID, serviceID, targetID uuid.UUID,
) (*model.Deployment, *model.Deployment, error) {
	var target, previous *model.Deployment
	err := conn.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		repo := s.projects.WithDB(tx)
		if _, err := repo.GetServiceByAccountForUpdate(ctx, accountID, projectID, serviceID); err != nil {
			return err
		}
		var err error
		target, err = repo.GetDeploymentByAccountForUpdate(ctx, accountID, projectID, serviceID, targetID)
		if err != nil {
			return err
		}
		if target.Status != model.DeploymentReady {
			return errors.New("rollback target is not a ready deployment")
		}
		previous, err = repo.GetActiveDeploymentForUpdate(ctx, accountID, projectID, serviceID)
		if errors.Is(err, sql.ErrNoRows) {
			previous = nil
			return nil
		}
		return err
	})
	return target, previous, err
}

func (s *DeploymentService) markRollbackActive(
	ctx context.Context,
	conn bun.Conn,
	accountID, projectID, serviceID, targetID uuid.UUID,
) (*model.Deployment, *model.Deployment, error) {
	var active, previous *model.Deployment
	err := conn.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		repo := s.projects.WithDB(tx)
		if _, err := repo.GetServiceByAccountForUpdate(ctx, accountID, projectID, serviceID); err != nil {
			return err
		}
		var err error
		active, previous, err = repo.ActivateReadyDeployment(ctx, accountID, projectID, serviceID, targetID)
		if err != nil {
			return err
		}
		if previous != nil && previous.ID == active.ID {
			return nil
		}
		if err := repo.AppendDeploymentEvent(ctx, accountID, projectID, serviceID, active.ID, "deployment.rollback_activated", "Previous healthy deployment activated", nil); err != nil {
			return err
		}
		if previous != nil {
			if err := repo.AppendDeploymentEvent(ctx, accountID, projectID, serviceID, previous.ID, "deployment.rolled_back", "Traffic rolled back to a previous deployment", nil); err != nil {
				return err
			}
		}
		return nil
	})
	return active, previous, err
}

func (s *DeploymentService) failDeployment(
	ctx context.Context,
	conn bun.Conn,
	deployment *model.Deployment,
	safeMessage string,
	cause error,
) error {
	persistErr := conn.RunInTx(context.WithoutCancel(ctx), nil, func(ctx context.Context, tx bun.Tx) error {
		repo := s.projects.WithDB(tx)
		if _, err := repo.GetServiceByAccountForUpdate(ctx, deployment.AccountID, deployment.ProjectID, deployment.ServiceID); err != nil {
			return err
		}
		current, err := repo.GetDeploymentByAccountForUpdate(ctx, deployment.AccountID, deployment.ProjectID, deployment.ServiceID, deployment.ID)
		if err != nil {
			return err
		}
		if current.Status == model.DeploymentReady || current.Status == model.DeploymentFailed || current.Status == model.DeploymentCancelled {
			return nil
		}
		if err := repo.MarkDeploymentFailed(ctx, current.ID, safeMessage); err != nil {
			return err
		}
		return repo.AppendDeploymentEvent(ctx, current.AccountID, current.ProjectID, current.ServiceID, current.ID, "deployment.failed", safeMessage, nil)
	})
	if persistErr != nil {
		return errors.Join(cause, fmt.Errorf("persist deployment failure: %w", persistErr))
	}
	return cause
}

func (s *DeploymentService) recordRetirementPending(
	ctx context.Context,
	conn bun.Conn,
	deployment *model.Deployment,
	message string,
) error {
	return conn.RunInTx(context.WithoutCancel(ctx), nil, func(ctx context.Context, tx bun.Tx) error {
		return s.projects.WithDB(tx).AppendDeploymentEvent(
			ctx, deployment.AccountID, deployment.ProjectID, deployment.ServiceID, deployment.ID,
			"deployment.retirement_pending", message, nil,
		)
	})
}

func (s *DeploymentService) requireStoredArtifact(ctx context.Context, artifact registry.Artifact) (registry.Artifact, error) {
	if err := artifact.Validate(); err != nil {
		return registry.Artifact{}, err
	}
	exists, err := s.registry.Exists(ctx, artifact)
	if err != nil {
		return registry.Artifact{}, fmt.Errorf("check private registry artifact: %w", err)
	}
	if !exists {
		return registry.Artifact{}, registry.ErrNotFound
	}
	resolved, err := s.registry.ResolveDigest(ctx, artifact.Repository, artifact.Digest)
	if err != nil {
		return registry.Artifact{}, fmt.Errorf("resolve private registry digest: %w", err)
	}
	if resolved.Digest != artifact.Digest {
		return registry.Artifact{}, registry.ErrDigestMismatch
	}
	if resolved.Repository != artifact.Repository {
		return registry.Artifact{}, errors.New("registry resolved an artifact outside its requested repository")
	}
	return resolved, nil
}

func artifactFromDeployment(deployment *model.Deployment) (registry.Artifact, error) {
	if deployment == nil || deployment.ImageSizeBytes == nil {
		return registry.Artifact{}, errors.New("deployment image metadata is incomplete")
	}
	repository, err := registry.ParseRepository(deployment.ImageReference)
	if err != nil {
		return registry.Artifact{}, err
	}
	artifact := registry.Artifact{Repository: repository, Digest: deployment.ImageDigest, SizeBytes: *deployment.ImageSizeBytes}
	if err := artifact.Validate(); err != nil {
		return registry.Artifact{}, err
	}
	if repository.AccountID != deployment.AccountID || repository.ProjectID != deployment.ProjectID || repository.ServiceID != deployment.ServiceID {
		return registry.Artifact{}, errors.New("deployment registry path does not match tenant scope")
	}
	return artifact, nil
}

func runtimeRevision(deployment *model.Deployment, artifact registry.Artifact) runtimeprovider.Revision {
	return runtimeprovider.Revision{
		AccountID: deployment.AccountID, ProjectID: deployment.ProjectID,
		ServiceID: deployment.ServiceID, DeploymentID: deployment.ID, Artifact: artifact,
	}
}

func (s *DeploymentService) withRuntimeLock(
	ctx context.Context,
	accountID, projectID, serviceID uuid.UUID,
	operation func(bun.Conn) error,
) (err error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve deployment worker connection: %w", err)
	}
	repo := s.projects.WithDB(conn)
	if err := repo.LockDeploymentServiceSession(ctx, accountID, projectID, serviceID); err != nil {
		return errors.Join(err, conn.Close())
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), deploymentUnlockTimeout)
		defer cancel()
		unlocked, unlockErr := repo.UnlockDeploymentServiceSession(unlockCtx, accountID, projectID, serviceID)
		if unlockErr != nil {
			err = errors.Join(err, fmt.Errorf("unlock deployment worker: %w", unlockErr))
		} else if !unlocked {
			err = errors.Join(err, errors.New("deployment worker lock was not held"))
		}
		err = errors.Join(err, conn.Close())
	}()
	return operation(conn)
}

func validatePublishRequest(
	accountID, projectID, serviceID uuid.UUID,
	request PublishDeploymentRequest,
) (string, *string, error) {
	if err := request.Repository.Validate(); err != nil {
		return "", nil, err
	}
	if request.Repository.AccountID != accountID || request.Repository.ProjectID != projectID || request.Repository.ServiceID != serviceID {
		return "", nil, errors.New("registry repository does not belong to deployment service")
	}
	if err := registry.ValidateDigest(request.SourceDigest); err != nil {
		return "", nil, err
	}
	if request.SourceBytes < 0 {
		return "", nil, errors.New("deployment image size cannot be negative")
	}
	provider := strings.ToLower(strings.TrimSpace(request.BuildProvider))
	if provider == "" || len(provider) > 100 {
		return "", nil, errors.New("build provider is invalid")
	}
	if request.SourceRevision == nil {
		return provider, nil, nil
	}
	revision := strings.TrimSpace(*request.SourceRevision)
	if revision == "" {
		return provider, nil, nil
	}
	if len(revision) > 512 {
		return "", nil, errors.New("source revision is too long")
	}
	return provider, &revision, nil
}
