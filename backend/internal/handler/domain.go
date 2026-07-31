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

// DomainHandler serves tenant-scoped custom-domain routes.
type DomainHandler struct {
	service *service.DomainService
}

// NewDomainHandler constructs a DomainHandler.
func NewDomainHandler(service *service.DomainService) *DomainHandler {
	return &DomainHandler{service: service}
}

type domainResponse struct {
	ID                    uuid.UUID  `json:"id"`
	SiteID                uuid.UUID  `json:"site_id"`
	Hostname              string     `json:"hostname"`
	Status                string     `json:"status"`
	VerificationExpiresAt time.Time  `json:"verification_expires_at"`
	VerifiedAt            *time.Time `json:"verified_at,omitempty"`
	DNSProvider           string     `json:"dns_provider"`
	CertStatus            string     `json:"cert_status"`
	CertExpiresAt         *time.Time `json:"cert_expires_at,omitempty"`
	CertObservedAt        *time.Time `json:"cert_observed_at,omitempty"`
	CertAutoRenew         bool       `json:"cert_auto_renew"`
	LastReconciledAt      *time.Time `json:"last_reconciled_at,omitempty"`
	LastError             *string    `json:"last_error,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// Attach handles POST /api/v1/sites/:id/domains.
func (h *DomainHandler) Attach(c *gin.Context) {
	siteID, ok := siteIDParam(c)
	if !ok {
		return
	}
	var request service.AttachDomainRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, apperr.Validation("invalid request body"))
		return
	}
	domain, err := h.service.Attach(
		c.Request.Context(),
		middleware.UserID(c),
		middleware.AccountID(c),
		siteID,
		c.GetHeader("Idempotency-Key"),
		request,
	)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": newDomainResponse(domain)})
}

// ListBySite handles GET /api/v1/sites/:id/domains.
func (h *DomainHandler) ListBySite(c *gin.Context) {
	siteID, ok := siteIDParam(c)
	if !ok {
		return
	}
	page, perPage := queryPagination(c)
	domains, total, err := h.service.ListBySite(
		c.Request.Context(), middleware.AccountID(c), siteID, page, perPage,
	)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": domainResponses(domains),
		"meta": gin.H{"page": page, "per_page": perPage, "total": total},
	})
}

// Get handles GET /api/v1/domains/:id.
func (h *DomainHandler) Get(c *gin.Context) {
	domainID, ok := domainIDParam(c)
	if !ok {
		return
	}
	domain, err := h.service.Get(
		c.Request.Context(), middleware.AccountID(c), domainID,
	)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": newDomainResponse(domain)})
}

// Instructions handles GET /api/v1/domains/:id/instructions.
func (h *DomainHandler) Instructions(c *gin.Context) {
	domainID, ok := domainIDParam(c)
	if !ok {
		return
	}
	instructions, err := h.service.Instructions(
		c.Request.Context(), middleware.AccountID(c), domainID,
	)
	if err != nil {
		respondError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("Pragma", "no-cache")
	c.JSON(http.StatusOK, gin.H{"data": instructions})
}

// RotateChallenge handles POST /api/v1/domains/:id/challenge.
func (h *DomainHandler) RotateChallenge(c *gin.Context) {
	domainID, ok := domainIDParam(c)
	if !ok {
		return
	}
	domain, err := h.service.RotateChallenge(
		c.Request.Context(), middleware.UserID(c), middleware.AccountID(c), domainID,
	)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": newDomainResponse(domain)})
}

// Verify handles POST /api/v1/domains/:id/verify.
func (h *DomainHandler) Verify(c *gin.Context) {
	domainID, ok := domainIDParam(c)
	if !ok {
		return
	}
	domain, err := h.service.Verify(
		c.Request.Context(), middleware.UserID(c), middleware.AccountID(c), domainID,
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
	domain, err := h.service.Detach(
		c.Request.Context(), middleware.UserID(c), middleware.AccountID(c), domainID,
	)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": newDomainResponse(domain)})
}

func domainIDParam(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, apperr.Validation("invalid domain id"))
		return uuid.Nil, false
	}
	return id, true
}

func domainResponses(domains []model.Domain) []domainResponse {
	responses := make([]domainResponse, len(domains))
	for index := range domains {
		responses[index] = newDomainResponse(&domains[index])
	}
	return responses
}

func newDomainResponse(domain *model.Domain) domainResponse {
	return domainResponse{
		ID: domain.ID, SiteID: domain.SiteID, Hostname: domain.Hostname,
		Status: domain.Status, VerificationExpiresAt: domain.VerificationExpiresAt,
		VerifiedAt: domain.VerifiedAt, DNSProvider: domain.DNSProvider,
		CertStatus: domain.CertStatus, CertExpiresAt: domain.CertExpiresAt,
		CertObservedAt: domain.CertObservedAt, CertAutoRenew: domain.CertAutoRenew,
		LastReconciledAt: domain.LastReconciledAt, LastError: domain.LastError,
		CreatedAt: domain.CreatedAt, UpdatedAt: domain.UpdatedAt,
	}
}
