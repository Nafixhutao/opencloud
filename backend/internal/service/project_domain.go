package service

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

var projectServiceNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// ProjectService owns tenant-scoped project and service mutations.
type ProjectService struct {
	db       *bun.DB
	projects *repository.ProjectRepo
	audit    *repository.AuditRepo
}

// NewProjectService constructs the tenant-scoped project-domain service.
func NewProjectService(db *bun.DB, projects *repository.ProjectRepo, audit *repository.AuditRepo) *ProjectService {
	return &ProjectService{db: db, projects: projects, audit: audit}
}

// CreateProjectRequest contains customer-controlled project creation fields.
type CreateProjectRequest struct {
	Name string `json:"name"`
}

// CreateServiceRequest contains customer-controlled service creation fields.
type CreateServiceRequest struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// CreateProject creates an idempotent tenant-owned project and audit event.
func (s *ProjectService) CreateProject(ctx context.Context, actor string, accountID uuid.UUID, key string, req CreateProjectRequest) (*model.Project, error) {
	name, key, err := validateProjectCreate(req, key)
	if err != nil {
		return nil, err
	}
	var created *model.Project
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		repo, audit := s.projects.WithDB(tx), s.audit.WithDB(tx)
		if err := repo.LockProjectCreateRequest(ctx, accountID, key); err != nil {
			return err
		}
		prior, priorErr := repo.GetProjectByIdempotencyKey(ctx, accountID, key)
		if priorErr == nil {
			if prior.Name != name {
				return apperr.Conflict("idempotency key was already used for another project")
			}
			created = prior
			return nil
		}
		if !errors.Is(priorErr, sql.ErrNoRows) {
			return priorErr
		}
		project := &model.Project{ID: uuid.New(), AccountID: accountID, Name: name, Status: model.ProjectActive, IdempotencyKey: &key}
		if err := repo.CreateProject(ctx, project); err != nil {
			return err
		}
		if err := appendProjectAudit(ctx, audit, accountID, actor, model.AuditProjectCreated, project.ID, map[string]any{"name": name}); err != nil {
			return err
		}
		created = project
		return nil
	})
	if err != nil {
		if apperr.As(err) != nil {
			return nil, err
		}
		if projectUniqueViolation(err) {
			return nil, apperr.Conflict("project name or idempotency key is already in use")
		}
		return nil, apperr.Internal("failed to create project").Wrap(err)
	}
	return created, nil
}

// ListProjects returns paginated projects owned by one account.
func (s *ProjectService) ListProjects(ctx context.Context, accountID uuid.UUID, page, perPage int) ([]model.Project, int, error) {
	page, perPage = canonicalProjectPage(page, perPage)
	rows, total, err := s.projects.ListProjects(ctx, accountID, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, apperr.Internal("failed to list projects").Wrap(err)
	}
	return rows, total, nil
}

// GetProject returns one project only when it belongs to the account.
func (s *ProjectService) GetProject(ctx context.Context, accountID, projectID uuid.UUID) (*model.Project, error) {
	project, err := s.projects.GetProjectByAccount(ctx, accountID, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("project not found")
	}
	if err != nil {
		return nil, apperr.Internal("failed to load project").Wrap(err)
	}
	return project, nil
}

// CreateService creates an idempotent service within one tenant-owned project.
func (s *ProjectService) CreateService(ctx context.Context, actor string, accountID, projectID uuid.UUID, key string, req CreateServiceRequest) (*model.Service, error) {
	name, serviceType, key, err := validateServiceCreate(req, key)
	if err != nil {
		return nil, err
	}
	var created *model.Service
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		repo, audit := s.projects.WithDB(tx), s.audit.WithDB(tx)
		if _, err := repo.GetProjectByAccountForUpdate(ctx, accountID, projectID); err != nil {
			return err
		}
		if err := repo.LockServiceCreateRequest(ctx, accountID, projectID, key); err != nil {
			return err
		}
		prior, priorErr := repo.GetServiceByIdempotencyKey(ctx, accountID, projectID, key)
		if priorErr == nil {
			if prior.Name != name || prior.ServiceType != serviceType {
				return apperr.Conflict("idempotency key was already used for another service")
			}
			created = prior
			return nil
		}
		if !errors.Is(priorErr, sql.ErrNoRows) {
			return priorErr
		}
		service := &model.Service{ID: uuid.New(), AccountID: accountID, ProjectID: projectID, Name: name, ServiceType: serviceType, SourceRoot: ".", Status: model.ServiceActive, IdempotencyKey: &key}
		if err := repo.CreateService(ctx, service); err != nil {
			return err
		}
		if err := appendProjectAudit(ctx, audit, accountID, actor, model.AuditProjectServiceCreated, service.ID, map[string]any{"project_id": projectID.String(), "name": name, "service_type": serviceType}); err != nil {
			return err
		}
		created = service
		return nil
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("project not found")
	}
	if err != nil {
		if apperr.As(err) != nil {
			return nil, err
		}
		if projectUniqueViolation(err) {
			return nil, apperr.Conflict("service name or idempotency key is already in use")
		}
		return nil, apperr.Internal("failed to create service").Wrap(err)
	}
	return created, nil
}

// ListServices returns paginated services for one tenant-owned project.
func (s *ProjectService) ListServices(ctx context.Context, accountID, projectID uuid.UUID, page, perPage int) ([]model.Service, int, error) {
	if _, err := s.GetProject(ctx, accountID, projectID); err != nil {
		return nil, 0, err
	}
	page, perPage = canonicalProjectPage(page, perPage)
	rows, total, err := s.projects.ListServicesByProject(ctx, accountID, projectID, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, apperr.Internal("failed to list services").Wrap(err)
	}
	return rows, total, nil
}

// GetService returns one service only when its project belongs to the account.
func (s *ProjectService) GetService(ctx context.Context, accountID, projectID, serviceID uuid.UUID) (*model.Service, error) {
	row, err := s.projects.GetServiceByAccount(ctx, accountID, projectID, serviceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("service not found")
	}
	if err != nil {
		return nil, apperr.Internal("failed to load service").Wrap(err)
	}
	return row, nil
}

// ListDeployments returns paginated immutable revisions for one tenant-owned service.
func (s *ProjectService) ListDeployments(ctx context.Context, accountID, projectID, serviceID uuid.UUID, page, perPage int) ([]model.Deployment, int, error) {
	if _, err := s.GetService(ctx, accountID, projectID, serviceID); err != nil {
		return nil, 0, err
	}
	page, perPage = canonicalProjectPage(page, perPage)
	rows, total, err := s.projects.ListDeploymentsByService(ctx, accountID, projectID, serviceID, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, apperr.Internal("failed to list deployments").Wrap(err)
	}
	return rows, total, nil
}

// GetDeployment returns one immutable revision only when its service is tenant-owned.
func (s *ProjectService) GetDeployment(ctx context.Context, accountID, projectID, serviceID, deploymentID uuid.UUID) (*model.Deployment, error) {
	row, err := s.projects.GetDeploymentByAccount(ctx, accountID, projectID, serviceID, deploymentID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("deployment not found")
	}
	if err != nil {
		return nil, apperr.Internal("failed to load deployment").Wrap(err)
	}
	return row, nil
}

// ListDeploymentEvents returns paginated safe events for one tenant-owned revision.
func (s *ProjectService) ListDeploymentEvents(ctx context.Context, accountID, projectID, serviceID, deploymentID uuid.UUID, page, perPage int) ([]model.DeploymentEvent, int, error) {
	if _, err := s.GetDeployment(ctx, accountID, projectID, serviceID, deploymentID); err != nil {
		return nil, 0, err
	}
	page, perPage = canonicalProjectPage(page, perPage)
	rows, total, err := s.projects.ListDeploymentEvents(ctx, accountID, projectID, serviceID, deploymentID, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, apperr.Internal("failed to list deployment events").Wrap(err)
	}
	return rows, total, nil
}

func appendProjectAudit(ctx context.Context, audit *repository.AuditRepo, accountID uuid.UUID, actor, action string, target uuid.UUID, metadata map[string]any) error {
	return audit.Append(ctx, repository.Entry{AccountID: &accountID, ActorID: &actor, Action: action, Target: projectStringPtr(target.String()), Metadata: metadata})
}

func validateProjectCreate(req CreateProjectRequest, idempotencyKey string) (string, string, error) {
	name := strings.TrimSpace(req.Name)
	if utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 100 {
		return "", "", apperr.Validation("invalid project name", apperr.FieldIssue{Field: "name", Issue: "must be between 1 and 100 characters"})
	}
	key, err := validateProjectIdempotencyKey(idempotencyKey)
	if err != nil {
		return "", "", err
	}
	return name, key, nil
}
func validateServiceCreate(req CreateServiceRequest, idempotencyKey string) (string, string, string, error) {
	name := strings.ToLower(strings.TrimSpace(req.Name))
	if !projectServiceNamePattern.MatchString(name) {
		return "", "", "", apperr.Validation("invalid service name", apperr.FieldIssue{Field: "name", Issue: "must use lowercase letters, numbers, and hyphens"})
	}
	serviceType := strings.ToLower(strings.TrimSpace(req.Type))
	if !isProjectServiceType(serviceType) {
		return "", "", "", apperr.Validation("invalid service type", apperr.FieldIssue{Field: "type", Issue: "must be web, worker, cron, or static"})
	}
	key, err := validateProjectIdempotencyKey(idempotencyKey)
	if err != nil {
		return "", "", "", err
	}
	return name, serviceType, key, nil
}
func validateProjectIdempotencyKey(value string) (string, error) {
	key := strings.TrimSpace(value)
	if key == "" {
		return "", apperr.Validation("idempotency key is required", apperr.FieldIssue{Field: "Idempotency-Key", Issue: "is required"})
	}
	if len(key) > 128 {
		return "", apperr.Validation("idempotency key is too long", apperr.FieldIssue{Field: "Idempotency-Key", Issue: "max 128"})
	}
	return key, nil
}
func isProjectServiceType(value string) bool {
	return value == model.ServiceTypeWeb || value == model.ServiceTypeWorker || value == model.ServiceTypeCron || value == model.ServiceTypeStatic
}
func canonicalProjectPage(page, perPage int) (int, int) {
	if page < 1 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}
	return page, perPage
}
func projectUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
func projectStringPtr(value string) *string { return &value }
