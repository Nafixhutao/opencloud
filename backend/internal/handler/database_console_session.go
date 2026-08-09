package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/middleware"
	"github.com/nazxf/opencloud/backend/internal/service"
)

// DatabaseConsoleSessionHandler handles database console session operations.
type DatabaseConsoleSessionHandler struct {
	sessionService *service.DatabaseConsoleSessionService
}

// NewDatabaseConsoleSessionHandler creates a new session handler.
func NewDatabaseConsoleSessionHandler(sessionService *service.DatabaseConsoleSessionService) *DatabaseConsoleSessionHandler {
	return &DatabaseConsoleSessionHandler{sessionService: sessionService}
}

// CreateSession generates a new database console session.
// POST /api/v1/databases/:databaseId/console/sessions
func (h *DatabaseConsoleSessionHandler) CreateSession(c *gin.Context) {
	databaseID := c.Param("databaseId")
	accountID := middleware.AccountID(c)
	userID := middleware.UserID(c)

	var req struct {
		TTLMinutes int `json:"ttlMinutes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperr.Validation("invalid request body"))
		return
	}

	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")

	result, err := h.sessionService.CreateSession(c.Request.Context(), service.CreateOptions{
		AccountID:  accountID.String(),
		ActorID:    userID,
		DatabaseID: databaseID,
		IPAddress:  ipAddress,
		UserAgent:  userAgent,
		TTLMinutes: req.TTLMinutes,
	})
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// RevokeSession terminates a session.
// DELETE /api/v1/databases/:databaseId/console/sessions/:sessionId
func (h *DatabaseConsoleSessionHandler) RevokeSession(c *gin.Context) {
	sessionID := c.Param("sessionId")
	accountID := middleware.AccountID(c)

	err := h.sessionService.RevokeSession(c.Request.Context(), accountID.String(), sessionID)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Session revoked successfully"})
}
