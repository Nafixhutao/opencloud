package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/middleware"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

// PreviewDeploymentHandler serves preview deployment endpoints.
type PreviewDeploymentHandler struct {
	repo *repository.PreviewDeploymentRepo
}

// NewPreviewDeploymentHandler constructs a preview deployment handler.
func NewPreviewDeploymentHandler(repo *repository.PreviewDeploymentRepo) *PreviewDeploymentHandler {
	return &PreviewDeploymentHandler{repo: repo}
}

// ListPreviews returns active preview deployments for a service.
func (h *PreviewDeploymentHandler) ListPreviews(c *gin.Context) {
	accountID := middleware.AccountID(c)
	serviceID, err := uuid.Parse(c.Param("serviceID"))
	if err != nil {
		respondError(c, apperr.Validation("invalid service id"))
		return
	}

	previews, err := h.repo.ListByService(c.Request.Context(), accountID, serviceID)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": previews})
}
