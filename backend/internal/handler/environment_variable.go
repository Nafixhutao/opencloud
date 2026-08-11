package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/dto"
	"github.com/nazxf/opencloud/backend/internal/middleware"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/service"
)

// EnvironmentVariableHandler handles environment variable and secret operations.
type EnvironmentVariableHandler struct {
	log    *zap.Logger
	envSvc *service.EnvironmentVariableService
}

// NewEnvironmentVariableHandler creates a handler for environment variables.
func NewEnvironmentVariableHandler(log *zap.Logger, envSvc *service.EnvironmentVariableService) *EnvironmentVariableHandler {
	return &EnvironmentVariableHandler{
		log:    log,
		envSvc: envSvc,
	}
}

// Create adds a new environment variable or secret.
// POST /api/v1/projects/:projectID/services/:serviceID/environment
func (h *EnvironmentVariableHandler) Create(c *gin.Context) {
	var req dto.CreateEnvironmentVariableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperr.Validation(err.Error()))
		return
	}

	accountID := middleware.AccountID(c)
	userID := middleware.UserID(c)
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		respondError(c, apperr.Validation("invalid project id"))
		return
	}
	serviceID, err := uuid.Parse(c.Param("serviceID"))
	if err != nil {
		respondError(c, apperr.Validation("invalid service id"))
		return
	}

	variable, err := h.envSvc.CreateVariable(
		c.Request.Context(),
		accountID,
		projectID,
		serviceID,
		userID,
		req.Key,
		req.Value,
		req.Environment,
		req.IsSecret,
	)
	if err != nil {
		respondError(c, err)
		return
	}

	// Redact secret value in response
	response := h.toResponse(variable)
	c.JSON(http.StatusCreated, gin.H{"data": response})
}

// Update modifies an existing environment variable or secret.
// PUT /api/v1/projects/:projectID/services/:serviceID/environment/:id
func (h *EnvironmentVariableHandler) Update(c *gin.Context) {
	var req dto.UpdateEnvironmentVariableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperr.Validation(err.Error()))
		return
	}

	accountID := middleware.AccountID(c)
	userID := middleware.UserID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, apperr.Validation("invalid variable id"))
		return
	}

	variable, err := h.envSvc.UpdateVariable(
		c.Request.Context(),
		accountID,
		userID,
		id,
		req.Value,
	)
	if err != nil {
		respondError(c, err)
		return
	}

	response := h.toResponse(variable)
	c.JSON(http.StatusOK, gin.H{"data": response})
}

// Delete removes an environment variable.
// DELETE /api/v1/projects/:projectID/services/:serviceID/environment/:id
func (h *EnvironmentVariableHandler) Delete(c *gin.Context) {
	accountID := middleware.AccountID(c)
	userID := middleware.UserID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, apperr.Validation("invalid variable id"))
		return
	}

	if err := h.envSvc.DeleteVariable(c.Request.Context(), accountID, userID, id); err != nil {
		respondError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// List retrieves all environment variables for a service and environment.
// GET /api/v1/projects/:projectID/services/:serviceID/environment
func (h *EnvironmentVariableHandler) List(c *gin.Context) {
	accountID := middleware.AccountID(c)
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		respondError(c, apperr.Validation("invalid project id"))
		return
	}
	serviceID, err := uuid.Parse(c.Param("serviceID"))
	if err != nil {
		respondError(c, apperr.Validation("invalid service id"))
		return
	}

	environment := strings.ToLower(strings.TrimSpace(c.Query("environment")))
	if environment == "" {
		environment = model.EnvProduction
	}

	variables, err := h.envSvc.ListVariables(
		c.Request.Context(),
		accountID,
		projectID,
		serviceID,
		environment,
	)
	if err != nil {
		respondError(c, err)
		return
	}

	responses := make([]dto.EnvironmentVariableResponse, len(variables))
	for i := range variables {
		responses[i] = h.toResponse(&variables[i])
	}

	c.JSON(http.StatusOK, gin.H{"data": responses})
}

// Reveal decrypts and returns a secret value with audit trail.
// POST /api/v1/projects/:projectID/services/:serviceID/environment/:id/reveal
func (h *EnvironmentVariableHandler) Reveal(c *gin.Context) {
	accountID := middleware.AccountID(c)
	userID := middleware.UserID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, apperr.Validation("invalid variable id"))
		return
	}

	value, err := h.envSvc.RevealSecret(c.Request.Context(), accountID, userID, id)
	if err != nil {
		respondError(c, err)
		return
	}

	// Never cache revealed secrets
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	c.JSON(http.StatusOK, gin.H{"data": dto.RevealSecretResponse{Value: value}})
}

// ListAudit retrieves audit trail for a service.
// GET /api/v1/projects/:projectID/services/:serviceID/environment/audit
func (h *EnvironmentVariableHandler) ListAudit(c *gin.Context) {
	accountID := middleware.AccountID(c)
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		respondError(c, apperr.Validation("invalid project id"))
		return
	}
	serviceID, err := uuid.Parse(c.Param("serviceID"))
	if err != nil {
		respondError(c, apperr.Validation("invalid service id"))
		return
	}

	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if _, err := fmt.Sscanf(limitStr, "%d", &limit); err != nil || limit <= 0 || limit > 100 {
			limit = 50
		}
	}

	audits, err := h.envSvc.ListAudit(c.Request.Context(), accountID, projectID, serviceID, limit)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": audits})
}

func (h *EnvironmentVariableHandler) toResponse(v *model.EnvironmentVariable) dto.EnvironmentVariableResponse {
	resp := dto.EnvironmentVariableResponse{
		ID:          v.ID,
		Key:         v.Key,
		IsSecret:    v.IsSecret,
		Environment: v.Environment,
		CreatedAt:   v.CreatedAt,
		UpdatedAt:   v.UpdatedAt,
	}
	// Only include value for non-secrets
	if !v.IsSecret && v.Value != nil {
		resp.Value = v.Value
	}
	return resp
}
