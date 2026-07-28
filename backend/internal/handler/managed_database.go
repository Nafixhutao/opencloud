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

// ManagedDatabaseHandler serves tenant-scoped asynchronous database routes.
type ManagedDatabaseHandler struct {
	svc *service.ManagedDatabaseService
}

type managedDatabaseResponse struct {
	ID                  uuid.UUID  `json:"id"`
	Name                string     `json:"name"`
	Engine              string     `json:"engine"`
	Status              string     `json:"status"`
	CredentialAvailable bool       `json:"credential_available"`
	LastError           *string    `json:"last_error,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	DeletedAt           *time.Time `json:"deleted_at,omitempty"`
}

// NewManagedDatabaseHandler constructs a ManagedDatabaseHandler.
func NewManagedDatabaseHandler(svc *service.ManagedDatabaseService) *ManagedDatabaseHandler {
	return &ManagedDatabaseHandler{svc: svc}
}

// List handles GET /api/v1/databases.
func (h *ManagedDatabaseHandler) List(c *gin.Context) {
	page, perPage := queryPagination(c)
	rows, total, err := h.svc.List(
		c.Request.Context(),
		middleware.AccountID(c),
		page,
		perPage,
	)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": managedDatabaseResponses(rows),
		"meta": gin.H{"page": page, "per_page": perPage, "total": total},
	})
}

// Create handles POST /api/v1/databases.
func (h *ManagedDatabaseHandler) Create(c *gin.Context) {
	var req service.CreateDatabaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperr.Validation("invalid request body"))
		return
	}
	row, err := h.svc.Create(
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
	c.JSON(http.StatusAccepted, gin.H{"data": newManagedDatabaseResponse(row)})
}

// Get handles GET /api/v1/databases/:id.
func (h *ManagedDatabaseHandler) Get(c *gin.Context) {
	databaseID, ok := managedDatabaseIDParam(c)
	if !ok {
		return
	}
	row, err := h.svc.Get(c.Request.Context(), middleware.AccountID(c), databaseID)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": newManagedDatabaseResponse(row)})
}

// Delete handles DELETE /api/v1/databases/:id.
func (h *ManagedDatabaseHandler) Delete(c *gin.Context) {
	databaseID, ok := managedDatabaseIDParam(c)
	if !ok {
		return
	}
	row, err := h.svc.Delete(
		c.Request.Context(),
		middleware.UserID(c),
		middleware.AccountID(c),
		databaseID,
	)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": newManagedDatabaseResponse(row)})
}

// RevealCredential handles POST /api/v1/databases/:id/credentials/reveal.
func (h *ManagedDatabaseHandler) RevealCredential(c *gin.Context) {
	databaseID, ok := managedDatabaseIDParam(c)
	if !ok {
		return
	}
	credentials, err := h.svc.RevealCredential(
		c.Request.Context(),
		middleware.UserID(c),
		middleware.AccountID(c),
		databaseID,
	)
	if err != nil {
		respondError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store, private")
	c.Header("Pragma", "no-cache")
	c.JSON(http.StatusOK, gin.H{"data": credentials})
}

func managedDatabaseResponses(rows []model.ManagedDatabase) []managedDatabaseResponse {
	out := make([]managedDatabaseResponse, len(rows))
	for i := range rows {
		out[i] = newManagedDatabaseResponse(&rows[i])
	}
	return out
}

func newManagedDatabaseResponse(row *model.ManagedDatabase) managedDatabaseResponse {
	return managedDatabaseResponse{
		ID:                  row.ID,
		Name:                row.Name,
		Engine:              row.Engine,
		Status:              row.Status,
		CredentialAvailable: row.CredentialAvailable,
		LastError:           row.LastError,
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
		DeletedAt:           row.DeletedAt,
	}
}

func managedDatabaseIDParam(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, apperr.Validation("invalid database id"))
		return uuid.Nil, false
	}
	return id, true
}
