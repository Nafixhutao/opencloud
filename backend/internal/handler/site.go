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

// SiteHandler serves tenant-scoped asynchronous site lifecycle routes.
type SiteHandler struct {
	svc *service.SiteService
}

type siteResponse struct {
	ID        uuid.UUID  `json:"id"`
	Domain    string     `json:"domain"`
	Status    string     `json:"status"`
	LastError *string    `json:"last_error,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

// NewSiteHandler constructs a SiteHandler.
func NewSiteHandler(svc *service.SiteService) *SiteHandler {
	return &SiteHandler{svc: svc}
}

// List handles GET /api/v1/sites.
func (h *SiteHandler) List(c *gin.Context) {
	page, perPage := queryPagination(c)
	sites, total, err := h.svc.List(c.Request.Context(), middleware.AccountID(c), page, perPage)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": siteResponses(sites),
		"meta": gin.H{"page": page, "per_page": perPage, "total": total},
	})
}

// Create handles POST /api/v1/sites.
func (h *SiteHandler) Create(c *gin.Context) {
	var req service.CreateSiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperr.Validation("invalid request body"))
		return
	}
	site, err := h.svc.Create(
		c.Request.Context(),
		middleware.UserID(c),
		middleware.AccountID(c),
		c.GetHeader("Idempotency-Key"),
		req,
	)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": newSiteResponse(site)})
}

// Get handles GET /api/v1/sites/:id.
func (h *SiteHandler) Get(c *gin.Context) {
	siteID, ok := siteIDParam(c)
	if !ok {
		return
	}
	site, err := h.svc.Get(c.Request.Context(), middleware.AccountID(c), siteID)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": newSiteResponse(site)})
}

// Suspend handles POST /api/v1/sites/:id/suspend.
func (h *SiteHandler) Suspend(c *gin.Context) {
	siteID, ok := siteIDParam(c)
	if !ok {
		return
	}
	site, err := h.svc.Suspend(
		c.Request.Context(),
		middleware.UserID(c),
		middleware.AccountID(c),
		siteID,
	)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": newSiteResponse(site)})
}

// Resume handles POST /api/v1/sites/:id/resume.
func (h *SiteHandler) Resume(c *gin.Context) {
	siteID, ok := siteIDParam(c)
	if !ok {
		return
	}
	site, err := h.svc.Resume(
		c.Request.Context(),
		middleware.UserID(c),
		middleware.AccountID(c),
		siteID,
	)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": newSiteResponse(site)})
}

// Delete handles DELETE /api/v1/sites/:id.
func (h *SiteHandler) Delete(c *gin.Context) {
	siteID, ok := siteIDParam(c)
	if !ok {
		return
	}
	site, err := h.svc.Delete(
		c.Request.Context(),
		middleware.UserID(c),
		middleware.AccountID(c),
		siteID,
	)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": newSiteResponse(site)})
}

func siteResponses(sites []model.Site) []siteResponse {
	out := make([]siteResponse, len(sites))
	for i := range sites {
		out[i] = newSiteResponse(&sites[i])
	}
	return out
}

func newSiteResponse(site *model.Site) siteResponse {
	return siteResponse{
		ID:        site.ID,
		Domain:    site.Domain,
		Status:    site.Status,
		LastError: site.LastError,
		CreatedAt: site.CreatedAt,
		UpdatedAt: site.UpdatedAt,
		DeletedAt: site.DeletedAt,
	}
}

func siteIDParam(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, apperr.Validation("invalid site id"))
		return uuid.Nil, false
	}
	return id, true
}
