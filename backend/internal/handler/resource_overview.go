package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/nazxf/opencloud/backend/internal/middleware"
	"github.com/nazxf/opencloud/backend/internal/service"
)

// ResourceOverviewHandler serves tenant-scoped dashboard aggregate reads.
type ResourceOverviewHandler struct {
	svc *service.ResourceOverviewService
}

// NewResourceOverviewHandler constructs a ResourceOverviewHandler.
func NewResourceOverviewHandler(
	svc *service.ResourceOverviewService,
) *ResourceOverviewHandler {
	return &ResourceOverviewHandler{svc: svc}
}

// Get handles GET /api/v1/overview.
func (h *ResourceOverviewHandler) Get(c *gin.Context) {
	overview, err := h.svc.Get(c.Request.Context(), middleware.AccountID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": overview})
}
