package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/middleware"
	"github.com/nazxf/opencloud/backend/internal/service"
)

// managedDatabaseIDParam extracts and validates the database ID parameter.
func managedDatabaseIDParam(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, apperr.Validation("invalid database id"))
		return uuid.Nil, false
	}
	return id, true
}

// DatabaseConsoleSessionHandler serves database console session routes.
type DatabaseConsoleSessionHandler struct {
	svc *service.DatabaseConsoleService
}

// NewDatabaseConsoleSessionHandler constructs a DatabaseConsoleSessionHandler.
func NewDatabaseConsoleSessionHandler(svc *service.DatabaseConsoleService) *DatabaseConsoleSessionHandler {
	return &DatabaseConsoleSessionHandler{svc: svc}
}

// ConsoleSessionResponse represents the response when creating a session.
type ConsoleSessionResponse struct {
	ID        uuid.UUID `json:"id"`
	ExpiresAt time.Time `json:"expires_at"`
	Token     string    `json:"token"`
}

// CreateSession handles POST /api/v1/databases/:id/console/session.
func (h *DatabaseConsoleSessionHandler) CreateSession(c *gin.Context) {
	databaseID, ok := managedDatabaseIDParam(c)
	if !ok {
		return
	}

	var req service.ConsoleSessionDurationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperr.Validation("invalid request body"))
		return
	}

	actorID := middleware.UserID(c)
	accountID := middleware.AccountID(c)

	session, err := h.svc.CreateSession(
		c.Request.Context(),
		actorID,
		accountID,
		databaseID,
		req.Duration,
	)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": newConsoleSessionResponse(session)})
}

// RevokeSession handles POST /api/v1/databases/:id/console/session/revoke.
func (h *DatabaseConsoleSessionHandler) RevokeSession(c *gin.Context) {
	databaseID, ok := managedDatabaseIDParam(c)
	if !ok {
		return
	}

	sessionID, ok := consoleSessionIDParam(c)
	if !ok {
		return
	}

	actorID := middleware.UserID(c)
	accountID := middleware.AccountID(c)

	err := h.svc.RevokeSession(
		c.Request.Context(),
		actorID,
		accountID,
		sessionID,
	)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

// Validate handles the gateway handshake. This endpoint is internal-only and
// called by db.<platform-domain> to authenticate console access.
func (h *DatabaseConsoleSessionHandler) Validate(c *gin.Context) {
	databaseID, err := uuid.Parse(c.Query("database_id"))
	if err != nil {
		c.String(http.StatusBadRequest, "invalid database_id")
		return
	}

	token := c.Query("token")
	if token == "" {
		c.String(http.StatusForbidden, "missing token")
		return
	}

	durationStr := c.Query("duration")
	duration := 30 * time.Minute // default
	if durationStr != "" {
		duration, err = time.ParseDuration(durationStr)
		if err != nil {
			c.String(http.StatusBadRequest, "invalid duration")
			return
		}
	}

	session, err := h.svc.ValidateSession(
		c.Request.Context(),
		databaseID,
		token,
	)
	if err != nil {
		if errors.Is(err, apperr.Forbidden) || errors.Is(err, apperr.Unauthenticated) {
			c.String(http.StatusForbidden, "unauthorized")
			return
		}
		c.String(http.StatusInternalServerError, "internal error")
		return
	}

	// Return minimal validation result - no sensitive data
	c.Header("X-OpenCloud-Console-Session", session.ID.String())
	c.Header("X-OpenCloud-Console-Engine", session.Engine)
	c.Header("X-OpenCloud-Console-Expires", session.ExpiresAt.Format(time.RFC3339))
	c.String(http.StatusOK, "ok")
}

func newConsoleSessionResponse(session *service.ConsoleSessionResponse) ConsoleSessionResponse {
	return ConsoleSessionResponse{
		ID:        session.ID,
		ExpiresAt: session.ExpiresAt,
		Token:     session.Token,
	}
}

func consoleSessionIDParam(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("session_id"))
	if err != nil {
		respondError(c, apperr.Validation("invalid session id"))
		return uuid.Nil, false
	}
	return id, true
}
