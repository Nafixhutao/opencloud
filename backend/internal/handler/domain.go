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

// DomainHandler serves tenant-scoped domain lifecycle routes.
type DomainHandler struct {
	svc *service.DomainService
}

type domainResponse struct {
	ID               uuid.UUID  `json:"id"`
	SiteID           *uuid.UUID `json:"site_id,omitempty"`
	Hostname         string     `json:"hostname"`
	Status           string     `json:"status"`
	VerificationType *string    `json:"verification_type,omitempty"`
	VerifiedAt       *time.Time `json:"verified_at,omitempty"`
	DNSProvider      string     `json:"dns_provider"`
	CertStatus       string     `json:"cert_status"`
	CertExpiresAt    *time.Time `json:"cert_expires_at,omitempty"`
	CertAutoRenew    bool       `json:"cert_auto_renew"`
	LastError        *string    `json:"last_error,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// NewDomainHandler constructs a DomainHandler.
func NewDomainHandler(svc *service.DomainService) *DomainHandler {
	return &DomainHandler{svc: svc}
}

// Attach handles POST /api/v1/sites/:id/domains.
func (h *DomainHandler) Attach(c *gin.Context) {
	siteID, ok := siteIDParam(c)
	if !ok {
		return
	}
	var req service.AttachDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperr.Validation("invalid request body"))
		return
	}
	domain, err := h.svc.Attach(
		c.Request.Context(),
		middleware.UserID(c),
		middleware.AccountID(c),
		siteID,
		req,
	)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": newDomainResponse(domain)})
}

// Get handles GET /api/v1/domains/:id.
func (h *DomainHandler) Get(c *gin.Context) {
	domainID, ok := domainIDParam(c)
	if !ok {
		return
	}
	domain, err := h.svc.Get(c.Request.Context(), middleware.AccountID(c), domainID)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": newDomainResponse(domain)})
}

// ListBySite handles GET /api/v1/sites/:id/domains.
func (h *DomainHandler) ListBySite(c *gin.Context) {
	siteID, ok := siteIDParam(c)
	if !ok {
		return
	}
	domains, err := h.svc.ListBySite(c.Request.Context(), middleware.AccountID(c), siteID)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": domainResponses(domains)})
}

// Verify handles POST /api/v1/domains/:id/verify.
func (h *DomainHandler) Verify(c *gin.Context) {
	domainID, ok := domainIDParam(c)
	if !ok {
		return
	}
	domain, err := h.svc.Verify(
		c.Request.Context(),
		middleware.UserID(c),
		middleware.AccountID(c),
		domainID,
	)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": newDomainResponse(domain)})
}

// Detach handles DELETE /api/v1/domains/:id.
func (h *DomainHandler) Detach(c *gin.Context) {
	domainID, ok := domainIDParam(c)
	if !ok {
		return
	}
	domain, err := h.svc.Detach(
		c.Request.Context(),
		middleware.UserID(c),
		middleware.AccountID(c),
		domainID,
	)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": newDomainResponse(domain)})
}

// GetInstructions handles GET /api/v1/domains/:id/instructions.
func (h *DomainHandler) GetInstructions(c *gin.Context) {
	domainID, ok := domainIDParam(c)
	if !ok {
		return
	}
	instructions, err := h.svc.GetInstructions(c.Request.Context(), middleware.AccountID(c), domainID)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": instructions})
}

func domainResponses(domains []model.Domain) []domainResponse {
	out := make([]domainResponse, len(domains))
	for i := range domains {
		out[i] = newDomainResponse(&domains[i])
	}
	return out
}

func newDomainResponse(domain *model.Domain) domainResponse {
	return domainResponse{
		ID:               domain.ID,
		SiteID:           domain.SiteID,
		Hostname:         domain.Hostname,
		Status:           domain.Status,
		VerificationType: domain.VerificationType,
		VerifiedAt:       domain.VerifiedAt,
		DNSProvider:      domain.DNSProvider,
		CertStatus:       domain.CertStatus,
		CertExpiresAt:    domain.CertExpiresAt,
		CertAutoRenew:    domain.CertAutoRenew,
		LastError:        domain.LastError,
		CreatedAt:        domain.CreatedAt,
		UpdatedAt:        domain.UpdatedAt,
	}
}

func domainIDParam(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, apperr.Validation("invalid domain id"))
		return uuid.Nil, false
	}
	return id, true
}