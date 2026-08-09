package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/middleware"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/service"
)

// ProjectHandler serves tenant-scoped Phase 4A resources.
type ProjectHandler struct{ svc *service.ProjectService }

// NewProjectHandler constructs a handler for tenant-scoped project resources.
func NewProjectHandler(svc *service.ProjectService) *ProjectHandler { return &ProjectHandler{svc: svc} }

type projectResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type serviceResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type deploymentResponse struct {
	ID             uuid.UUID `json:"id"`
	Revision       int       `json:"revision"`
	ImageDigest    string    `json:"image_digest"`
	BuildProvider  string    `json:"build_provider"`
	SourceRevision *string   `json:"source_revision,omitempty"`
	Status         string    `json:"status"`
	IsActive       bool      `json:"is_active"`
	LastError      *string   `json:"last_error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
type deploymentEventResponse struct {
	ID        uuid.UUID `json:"id"`
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// ListProjects returns the current account's paginated projects.
func (h *ProjectHandler) ListProjects(c *gin.Context) {
	page, perPage := queryPagination(c)
	rows, total, err := h.svc.ListProjects(c.Request.Context(), middleware.AccountID(c), page, perPage)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": projectResponses(rows), "meta": gin.H{"page": page, "per_page": perPage, "total": total}})
}

// CreateProject creates a project for the current account.
func (h *ProjectHandler) CreateProject(c *gin.Context) {
	var req service.CreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperr.Validation("invalid request body"))
		return
	}
	row, err := h.svc.CreateProject(c.Request.Context(), middleware.UserID(c), middleware.AccountID(c), c.GetHeader("Idempotency-Key"), req)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": newProjectResponse(row)})
}

// GetProject returns one account-owned project.
func (h *ProjectHandler) GetProject(c *gin.Context) {
	projectID, ok := projectIDParam(c)
	if !ok {
		return
	}
	row, err := h.svc.GetProject(c.Request.Context(), middleware.AccountID(c), projectID)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": newProjectResponse(row)})
}

// ListServices returns the paginated services for one account-owned project.
func (h *ProjectHandler) ListServices(c *gin.Context) {
	projectID, ok := projectIDParam(c)
	if !ok {
		return
	}
	page, perPage := queryPagination(c)
	rows, total, err := h.svc.ListServices(c.Request.Context(), middleware.AccountID(c), projectID, page, perPage)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serviceResponses(rows), "meta": gin.H{"page": page, "per_page": perPage, "total": total}})
}

// CreateService creates a service within one account-owned project.
func (h *ProjectHandler) CreateService(c *gin.Context) {
	projectID, ok := projectIDParam(c)
	if !ok {
		return
	}
	var req service.CreateServiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperr.Validation("invalid request body"))
		return
	}
	row, err := h.svc.CreateService(c.Request.Context(), middleware.UserID(c), middleware.AccountID(c), projectID, c.GetHeader("Idempotency-Key"), req)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": newServiceResponse(row)})
}

// GetService returns one account-owned project service.
func (h *ProjectHandler) GetService(c *gin.Context) {
	projectID, serviceID, ok := projectServiceParams(c)
	if !ok {
		return
	}
	row, err := h.svc.GetService(c.Request.Context(), middleware.AccountID(c), projectID, serviceID)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": newServiceResponse(row)})
}

// ListDeployments returns the paginated revisions for one account-owned service.
func (h *ProjectHandler) ListDeployments(c *gin.Context) {
	projectID, serviceID, ok := projectServiceParams(c)
	if !ok {
		return
	}
	page, perPage := queryPagination(c)
	rows, total, err := h.svc.ListDeployments(c.Request.Context(), middleware.AccountID(c), projectID, serviceID, page, perPage)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": deploymentResponses(rows), "meta": gin.H{"page": page, "per_page": perPage, "total": total}})
}

// GetDeployment returns one account-owned immutable deployment revision.
func (h *ProjectHandler) GetDeployment(c *gin.Context) {
	projectID, serviceID, deploymentID, ok := projectDeploymentParams(c)
	if !ok {
		return
	}
	row, err := h.svc.GetDeployment(c.Request.Context(), middleware.AccountID(c), projectID, serviceID, deploymentID)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": newDeploymentResponse(row)})
}

// ListDeploymentEvents returns the paginated safe events for one deployment.
func (h *ProjectHandler) ListDeploymentEvents(c *gin.Context) {
	projectID, serviceID, deploymentID, ok := projectDeploymentParams(c)
	if !ok {
		return
	}
	page, perPage := queryPagination(c)
	rows, total, err := h.svc.ListDeploymentEvents(c.Request.Context(), middleware.AccountID(c), projectID, serviceID, deploymentID, page, perPage)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": deploymentEventResponses(rows), "meta": gin.H{"page": page, "per_page": perPage, "total": total}})
}

func newProjectResponse(row *model.Project) projectResponse {
	return projectResponse{ID: row.ID, Name: row.Name, Status: row.Status, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}
func projectResponses(rows []model.Project) []projectResponse {
	out := make([]projectResponse, len(rows))
	for i := range rows {
		out[i] = newProjectResponse(&rows[i])
	}
	return out
}
func newServiceResponse(row *model.Service) serviceResponse {
	return serviceResponse{ID: row.ID, Name: row.Name, Type: row.ServiceType, Status: row.Status, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}
func serviceResponses(rows []model.Service) []serviceResponse {
	out := make([]serviceResponse, len(rows))
	for i := range rows {
		out[i] = newServiceResponse(&rows[i])
	}
	return out
}
func newDeploymentResponse(row *model.Deployment) deploymentResponse {
	return deploymentResponse{ID: row.ID, Revision: row.Revision, ImageDigest: row.ImageDigest, BuildProvider: row.BuildProvider, SourceRevision: row.SourceRevision, Status: row.Status, IsActive: row.IsActive, LastError: row.LastError, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}
func deploymentResponses(rows []model.Deployment) []deploymentResponse {
	out := make([]deploymentResponse, len(rows))
	for i := range rows {
		out[i] = newDeploymentResponse(&rows[i])
	}
	return out
}
func deploymentEventResponses(rows []model.DeploymentEvent) []deploymentEventResponse {
	out := make([]deploymentEventResponse, len(rows))
	for i := range rows {
		out[i] = deploymentEventResponse{ID: rows[i].ID, Type: rows[i].EventType, Message: rows[i].Message, CreatedAt: rows[i].CreatedAt}
	}
	return out
}
func projectIDParam(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		respondError(c, apperr.Validation("invalid project id"))
		return uuid.Nil, false
	}
	return id, true
}
func projectServiceParams(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	projectID, ok := projectIDParam(c)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	serviceID, err := uuid.Parse(c.Param("serviceID"))
	if err != nil {
		respondError(c, apperr.Validation("invalid service id"))
		return uuid.Nil, uuid.Nil, false
	}
	return projectID, serviceID, true
}
func projectDeploymentParams(c *gin.Context) (uuid.UUID, uuid.UUID, uuid.UUID, bool) {
	projectID, serviceID, ok := projectServiceParams(c)
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	deploymentID, err := uuid.Parse(c.Param("deploymentID"))
	if err != nil {
		respondError(c, apperr.Validation("invalid deployment id"))
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	return projectID, serviceID, deploymentID, true
}
