package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nazxf/opencloud/backend/internal/service"
)

// DatabaseConsoleSessionHandler handles database console session operations
type DatabaseConsoleSessionHandler struct {
	sessionService *service.DatabaseConsoleSessionService
}

// NewDatabaseConsoleSessionHandler creates a new session handler
func NewDatabaseConsoleSessionHandler(sessionService *service.DatabaseConsoleSessionService) *DatabaseConsoleSessionHandler {
	return &DatabaseConsoleSessionHandler{sessionService: sessionService}
}

// CreateSession generates a new database console session
// POST /api/v1/databases/:databaseId/console/sessions
func (h *DatabaseConsoleSessionHandler) CreateSession(c *gin.Context) {
	databaseID := c.Param("databaseId")
	accountID := c.GetString("account_id")

	ttlMinutes := 30 // Default TTL

	result, err := h.sessionService.CreateSession(c.Request.Context(), service.CreateOptions{
		AccountID:  accountID,
		DatabaseID: databaseID,
		TTLMinutes: ttlMinutes,
	})

	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": result,
	})
}

// RevokeSession terminates a session
// DELETE /api/v1/databases/:databaseId/console/sessions/:sessionId
func (h *DatabaseConsoleSessionHandler) RevokeSession(c *gin.Context) {
	sessionID := c.Param("sessionId")
	accountID := c.GetString("account_id")

	err := h.sessionService.RevokeSession(c.Request.Context(), accountID, sessionID)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Session revoked successfully"})
}
