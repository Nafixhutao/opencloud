package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/model"
)

// ProjectRepo manages tenant-owned projects, their services, and safe reads of
// deployment state. Every customer-facing method requires accountID.
type ProjectRepo struct {
	db bun.IDB
}

// NewProjectRepo constructs a ProjectRepo.
func NewProjectRepo(db bun.IDB) *ProjectRepo {
	return &ProjectRepo{db: db}
}

// WithDB returns a copy using db, normally a transaction owned by a service.
func (r *ProjectRepo) WithDB(db bun.IDB) *ProjectRepo {
	return &ProjectRepo{db: db}
}

// LockProjectCreateRequest serializes idempotent retries for one account.
func (r *ProjectRepo) LockProjectCreateRequest(
	ctx context.Context,
	accountID uuid.UUID,
	idempotencyKey string,
) error {
	_, err := r.db.NewRaw(
		`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`,
		"project:"+accountID.String()+":"+idempotencyKey,
	).Exec(ctx)
	return err
}

// LockServiceCreateRequest serializes idempotent service creation within a project.
func (r *ProjectRepo) LockServiceCreateRequest(
	ctx context.Context,
	accountID, projectID uuid.UUID,
	idempotencyKey string,
) error {
	_, err := r.db.NewRaw(
		`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`,
		"project-service:"+accountID.String()+":"+projectID.String()+":"+idempotencyKey,
	).Exec(ctx)
	return err
}

// CreateProject inserts a tenant-owned project.
func (r *ProjectRepo) CreateProject(ctx context.Context, project *model.Project) error {
	now := time.Now().UTC()
	if project.ID == uuid.Nil {
		project.ID = uuid.New()
	}
	project.CreatedAt = now
	project.UpdatedAt = now
	_, err := r.db.NewInsert().Model(project).Exec(ctx)
	return err
}

// GetProjectByIdempotencyKey returns a prior creation result for a safe retry.
func (r *ProjectRepo) GetProjectByIdempotencyKey(
	ctx context.Context,
	accountID uuid.UUID,
	idempotencyKey string,
) (*model.Project, error) {
	project := new(model.Project)
	err := r.db.NewSelect().Model(project).
		Where("account_id = ?", accountID).
		Where("idempotency_key = ?", idempotencyKey).
		Scan(ctx)
	return project, err
}

// ListProjects returns live projects owned by accountID.
func (r *ProjectRepo) ListProjects(
	ctx context.Context,
	accountID uuid.UUID,
	limit, offset int,
) ([]model.Project, int, error) {
	total, err := r.db.NewSelect().
		Model((*model.Project)(nil)).
		Where("account_id = ?", accountID).
		Where("deleted_at IS NULL").
		Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	var projects []model.Project
	err = r.db.NewSelect().Model(&projects).
		Where("account_id = ?", accountID).
		Where("deleted_at IS NULL").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(ctx)
	return projects, total, err
}

// GetProjectByAccount returns a live project only when the tenant owns it.
func (r *ProjectRepo) GetProjectByAccount(
	ctx context.Context,
	accountID, projectID uuid.UUID,
) (*model.Project, error) {
	project := new(model.Project)
	err := r.db.NewSelect().Model(project).
		Where("account_id = ?", accountID).
		Where("id = ?", projectID).
		Where("deleted_at IS NULL").
		Scan(ctx)
	return project, err
}

// GetProjectByAccountForUpdate locks a live project while a service is added.
func (r *ProjectRepo) GetProjectByAccountForUpdate(
	ctx context.Context,
	accountID, projectID uuid.UUID,
) (*model.Project, error) {
	project := new(model.Project)
	err := r.db.NewSelect().Model(project).
		Where("account_id = ?", accountID).
		Where("id = ?", projectID).
		Where("deleted_at IS NULL").
		For("UPDATE").
		Scan(ctx)
	return project, err
}

// CreateService inserts one independently deployable project workload.
func (r *ProjectRepo) CreateService(ctx context.Context, service *model.Service) error {
	now := time.Now().UTC()
	if service.ID == uuid.Nil {
		service.ID = uuid.New()
	}
	service.CreatedAt = now
	service.UpdatedAt = now
	_, err := r.db.NewInsert().Model(service).Exec(ctx)
	return err
}

// GetServiceByIdempotencyKey returns a prior service creation result.
func (r *ProjectRepo) GetServiceByIdempotencyKey(
	ctx context.Context,
	accountID, projectID uuid.UUID,
	idempotencyKey string,
) (*model.Service, error) {
	service := new(model.Service)
	err := r.db.NewSelect().Model(service).
		Where("account_id = ?", accountID).
		Where("project_id = ?", projectID).
		Where("idempotency_key = ?", idempotencyKey).
		Scan(ctx)
	return service, err
}

// ListServicesByProject returns live services only for the owning tenant/project.
func (r *ProjectRepo) ListServicesByProject(
	ctx context.Context,
	accountID, projectID uuid.UUID,
	limit, offset int,
) ([]model.Service, int, error) {
	total, err := r.db.NewSelect().
		Model((*model.Service)(nil)).
		Where("account_id = ?", accountID).
		Where("project_id = ?", projectID).
		Where("deleted_at IS NULL").
		Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	var services []model.Service
	err = r.db.NewSelect().Model(&services).
		Where("account_id = ?", accountID).
		Where("project_id = ?", projectID).
		Where("deleted_at IS NULL").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(ctx)
	return services, total, err
}

// GetServiceByAccount returns a live service only under its correct project.
func (r *ProjectRepo) GetServiceByAccount(
	ctx context.Context,
	accountID, projectID, serviceID uuid.UUID,
) (*model.Service, error) {
	service := new(model.Service)
	err := r.db.NewSelect().Model(service).
		Where("account_id = ?", accountID).
		Where("project_id = ?", projectID).
		Where("id = ?", serviceID).
		Where("deleted_at IS NULL").
		Scan(ctx)
	return service, err
}

// GetServiceByAccountForUpdate locks one live service for a deployment
// mutation. The caller must use a transaction and still take the service-level
// advisory lock before it performs external runtime work.
func (r *ProjectRepo) GetServiceByAccountForUpdate(
	ctx context.Context,
	accountID, projectID, serviceID uuid.UUID,
) (*model.Service, error) {
	service := new(model.Service)
	err := r.db.NewSelect().Model(service).
		Where("account_id = ?", accountID).
		Where("project_id = ?", projectID).
		Where("id = ?", serviceID).
		Where("deleted_at IS NULL").
		For("UPDATE").
		Scan(ctx)
	return service, err
}

// LockDeploymentService serializes revision allocation and activation for one
// service across workers without holding a SQL transaction during runtime I/O.
func (r *ProjectRepo) LockDeploymentService(
	ctx context.Context,
	accountID, projectID, serviceID uuid.UUID,
) error {
	_, err := r.db.NewRaw(
		`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`,
		"deployment:"+accountID.String()+":"+projectID.String()+":"+serviceID.String(),
	).Exec(ctx)
	return err
}

// LockDeploymentServiceSession holds the deployment lock across runtime start,
// health, and Caddy work. Callers must use one dedicated bun.Conn and pair it
// with UnlockDeploymentServiceSession before returning it to the pool.
func (r *ProjectRepo) LockDeploymentServiceSession(
	ctx context.Context,
	accountID, projectID, serviceID uuid.UUID,
) error {
	_, err := r.db.NewRaw(
		`SELECT pg_advisory_lock(hashtextextended(?, 0))`,
		"deployment:"+accountID.String()+":"+projectID.String()+":"+serviceID.String(),
	).Exec(ctx)
	return err
}

// UnlockDeploymentServiceSession releases a corresponding session lock.
func (r *ProjectRepo) UnlockDeploymentServiceSession(
	ctx context.Context,
	accountID, projectID, serviceID uuid.UUID,
) (bool, error) {
	var unlocked bool
	err := r.db.NewRaw(
		`SELECT pg_advisory_unlock(hashtextextended(?, 0))`,
		"deployment:"+accountID.String()+":"+projectID.String()+":"+serviceID.String(),
	).Scan(ctx, &unlocked)
	return unlocked, err
}

// NextDeploymentRevision allocates the next monotonically increasing revision
// while the caller holds LockDeploymentService.
func (r *ProjectRepo) NextDeploymentRevision(
	ctx context.Context,
	accountID, projectID, serviceID uuid.UUID,
) (int, error) {
	var revision int
	err := r.db.NewRaw(`
		SELECT COALESCE(MAX(revision), 0) + 1
		FROM deployments
		WHERE account_id = ? AND project_id = ? AND service_id = ?`,
		accountID, projectID, serviceID,
	).Scan(ctx, &revision)
	return revision, err
}

// CreateDeployment inserts an immutable deployment identity. Status, active
// marker, timestamps, and safe error can advance later; the database trigger
// rejects identity mutations.
func (r *ProjectRepo) CreateDeployment(ctx context.Context, deployment *model.Deployment) error {
	now := time.Now().UTC()
	if deployment.ID == uuid.Nil {
		deployment.ID = uuid.New()
	}
	deployment.CreatedAt = now
	deployment.UpdatedAt = now
	_, err := r.db.NewInsert().Model(deployment).Exec(ctx)
	return err
}

// AppendDeploymentEvent writes one safe append-only customer activity record.
// Callers must pass metadata that is already safe for a tenant to view.
func (r *ProjectRepo) AppendDeploymentEvent(
	ctx context.Context,
	accountID, projectID, serviceID, deploymentID uuid.UUID,
	eventType, message string,
	metadata map[string]any,
) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal deployment event metadata: %w", err)
	}
	event := &model.DeploymentEvent{
		ID: uuid.New(), AccountID: accountID, ProjectID: projectID, ServiceID: serviceID,
		DeploymentID: deploymentID, EventType: eventType, Message: message, Metadata: raw,
		CreatedAt: time.Now().UTC(),
	}
	_, err = r.db.NewInsert().Model(event).Exec(ctx)
	return err
}

// GetDeploymentByAccountForUpdate locks one tenant-owned deployment while its
// status or active marker changes.
func (r *ProjectRepo) GetDeploymentByAccountForUpdate(
	ctx context.Context,
	accountID, projectID, serviceID, deploymentID uuid.UUID,
) (*model.Deployment, error) {
	deployment := new(model.Deployment)
	err := r.db.NewSelect().Model(deployment).
		Where("account_id = ?", accountID).
		Where("project_id = ?", projectID).
		Where("service_id = ?", serviceID).
		Where("id = ?", deploymentID).
		For("UPDATE").
		Scan(ctx)
	return deployment, err
}

// GetActiveDeploymentForUpdate locks the active revision when one exists.
func (r *ProjectRepo) GetActiveDeploymentForUpdate(
	ctx context.Context,
	accountID, projectID, serviceID uuid.UUID,
) (*model.Deployment, error) {
	deployment := new(model.Deployment)
	err := r.db.NewSelect().Model(deployment).
		Where("account_id = ?", accountID).
		Where("project_id = ?", projectID).
		Where("service_id = ?", serviceID).
		Where("is_active = TRUE").
		For("UPDATE").
		Scan(ctx)
	return deployment, err
}

// MarkDeploymentDeploying records that the immutable artifact is now in the
// restricted runtime worker. It may start only after registry publication.
func (r *ProjectRepo) MarkDeploymentDeploying(ctx context.Context, deploymentID uuid.UUID) error {
	result, err := r.db.NewUpdate().Model((*model.Deployment)(nil)).
		Set("status = ?", model.DeploymentDeploying).
		Set("started_at = COALESCE(started_at, now())").
		Set("last_error = NULL").
		Set("updated_at = now()").
		Where("id = ?", deploymentID).
		Where("status IN (?)", bun.In([]string{model.DeploymentPushing, model.DeploymentScanning})).
		Exec(ctx)
	if err != nil {
		return err
	}
	return requireOneRow(result)
}

// MarkDeploymentFailed marks a nonterminal deployment failed with a short,
// pre-sanitized customer-safe error. The worker logs detailed causes privately.
func (r *ProjectRepo) MarkDeploymentFailed(ctx context.Context, deploymentID uuid.UUID, safeError string) error {
	result, err := r.db.NewUpdate().Model((*model.Deployment)(nil)).
		Set("status = ?", model.DeploymentFailed).
		Set("is_active = FALSE").
		Set("last_error = ?", safeError).
		Set("completed_at = now()").
		Set("updated_at = now()").
		Where("id = ?", deploymentID).
		Where("status NOT IN (?)", bun.In([]string{model.DeploymentReady, model.DeploymentFailed, model.DeploymentCancelled})).
		Exec(ctx)
	if err != nil {
		return err
	}
	return requireOneRow(result)
}

// ActivateDeployment atomically promotes one healthy candidate and clears the
// old active marker. The caller holds the service lock and has already switched
// Caddy traffic; this function never changes immutable image fields.
func (r *ProjectRepo) ActivateDeployment(
	ctx context.Context,
	accountID, projectID, serviceID, deploymentID uuid.UUID,
) (*model.Deployment, *model.Deployment, error) {
	candidate, err := r.GetDeploymentByAccountForUpdate(ctx, accountID, projectID, serviceID, deploymentID)
	if err != nil {
		return nil, nil, err
	}
	if candidate.Status != model.DeploymentDeploying {
		return nil, nil, fmt.Errorf("deployment is not deploying")
	}
	previous, previousErr := r.GetActiveDeploymentForUpdate(ctx, accountID, projectID, serviceID)
	if previousErr != nil && !errors.Is(previousErr, sql.ErrNoRows) {
		return nil, nil, previousErr
	}
	if previousErr == nil {
		result, err := r.db.NewUpdate().Model((*model.Deployment)(nil)).
			Set("is_active = FALSE").
			Set("updated_at = now()").
			Where("id = ?", previous.ID).
			Where("is_active = TRUE").
			Exec(ctx)
		if err != nil {
			return nil, nil, err
		}
		if err := requireOneRow(result); err != nil {
			return nil, nil, err
		}
	}

	result, err := r.db.NewUpdate().Model((*model.Deployment)(nil)).
		Set("status = ?", model.DeploymentReady).
		Set("is_active = TRUE").
		Set("last_error = NULL").
		Set("ready_at = now()").
		Set("completed_at = now()").
		Set("updated_at = now()").
		Where("id = ?", candidate.ID).
		Where("status = ?", model.DeploymentDeploying).
		Exec(ctx)
	if err != nil {
		return nil, nil, err
	}
	if err := requireOneRow(result); err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	candidate.Status, candidate.IsActive, candidate.LastError = model.DeploymentReady, true, nil
	candidate.ReadyAt, candidate.CompletedAt, candidate.UpdatedAt = &now, &now, now
	if previousErr == nil {
		previous.IsActive = false
		previous.UpdatedAt = now
		return candidate, previous, nil
	}
	return candidate, nil, nil
}

// ActivateReadyDeployment switches the active marker to a previously healthy
// immutable revision during rollback. Unlike ActivateDeployment it never
// changes status, image identity, or terminal timestamps.
func (r *ProjectRepo) ActivateReadyDeployment(
	ctx context.Context,
	accountID, projectID, serviceID, deploymentID uuid.UUID,
) (*model.Deployment, *model.Deployment, error) {
	candidate, err := r.GetDeploymentByAccountForUpdate(ctx, accountID, projectID, serviceID, deploymentID)
	if err != nil {
		return nil, nil, err
	}
	if candidate.Status != model.DeploymentReady {
		return nil, nil, fmt.Errorf("rollback target is not ready")
	}
	previous, previousErr := r.GetActiveDeploymentForUpdate(ctx, accountID, projectID, serviceID)
	if previousErr != nil && !errors.Is(previousErr, sql.ErrNoRows) {
		return nil, nil, previousErr
	}
	if previousErr == nil && previous.ID == candidate.ID {
		return candidate, previous, nil
	}
	if previousErr == nil {
		result, err := r.db.NewUpdate().Model((*model.Deployment)(nil)).
			Set("is_active = FALSE").
			Set("updated_at = now()").
			Where("id = ?", previous.ID).
			Where("is_active = TRUE").
			Exec(ctx)
		if err != nil {
			return nil, nil, err
		}
		if err := requireOneRow(result); err != nil {
			return nil, nil, err
		}
	}
	result, err := r.db.NewUpdate().Model((*model.Deployment)(nil)).
		Set("is_active = TRUE").
		Set("updated_at = now()").
		Where("id = ?", candidate.ID).
		Where("status = ?", model.DeploymentReady).
		Exec(ctx)
	if err != nil {
		return nil, nil, err
	}
	if err := requireOneRow(result); err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	candidate.IsActive, candidate.UpdatedAt = true, now
	if previousErr == nil {
		previous.IsActive, previous.UpdatedAt = false, now
		return candidate, previous, nil
	}
	return candidate, nil, nil
}

// ListDeploymentsByService returns immutable deployment revisions for a service.
func (r *ProjectRepo) ListDeploymentsByService(
	ctx context.Context,
	accountID, projectID, serviceID uuid.UUID,
	limit, offset int,
) ([]model.Deployment, int, error) {
	total, err := r.db.NewSelect().
		Model((*model.Deployment)(nil)).
		Where("account_id = ?", accountID).
		Where("project_id = ?", projectID).
		Where("service_id = ?", serviceID).
		Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	var deployments []model.Deployment
	err = r.db.NewSelect().Model(&deployments).
		Where("account_id = ?", accountID).
		Where("project_id = ?", projectID).
		Where("service_id = ?", serviceID).
		Order("revision DESC").
		Limit(limit).
		Offset(offset).
		Scan(ctx)
	return deployments, total, err
}

// GetDeploymentByAccount returns one immutable revision under its service.
func (r *ProjectRepo) GetDeploymentByAccount(
	ctx context.Context,
	accountID, projectID, serviceID, deploymentID uuid.UUID,
) (*model.Deployment, error) {
	deployment := new(model.Deployment)
	err := r.db.NewSelect().Model(deployment).
		Where("account_id = ?", accountID).
		Where("project_id = ?", projectID).
		Where("service_id = ?", serviceID).
		Where("id = ?", deploymentID).
		Scan(ctx)
	return deployment, err
}

// ListDeploymentEvents returns the append-only activity stream for a revision.
func (r *ProjectRepo) ListDeploymentEvents(
	ctx context.Context,
	accountID, projectID, serviceID, deploymentID uuid.UUID,
	limit, offset int,
) ([]model.DeploymentEvent, int, error) {
	total, err := r.db.NewSelect().
		Model((*model.DeploymentEvent)(nil)).
		Where("account_id = ?", accountID).
		Where("project_id = ?", projectID).
		Where("service_id = ?", serviceID).
		Where("deployment_id = ?", deploymentID).
		Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	var events []model.DeploymentEvent
	err = r.db.NewSelect().Model(&events).
		Where("account_id = ?", accountID).
		Where("project_id = ?", projectID).
		Where("service_id = ?", serviceID).
		Where("deployment_id = ?", deploymentID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(ctx)
	return events, total, err
}
