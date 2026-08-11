package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/middleware"
	"github.com/nazxf/opencloud/backend/internal/service"
)

// ConsoleQueryHandler handles SQL query execution operations.
type ConsoleQueryHandler struct {
	queryService *service.ConsoleQueryService
}

// NewConsoleQueryHandler creates a new query handler.
func NewConsoleQueryHandler(queryService *service.ConsoleQueryService) *ConsoleQueryHandler {
	return &ConsoleQueryHandler{queryService: queryService}
}

// ExecuteQuery executes a read-only SQL query in the database console.
// POST /api/v1/databases/:id/console/execute
func (h *ConsoleQueryHandler) ExecuteQuery(c *gin.Context) {
	databaseID := c.Param("id")
	accountID := middleware.AccountID(c)
	userID := middleware.UserID(c)

	var req struct {
		SessionID         string `json:"sessionId" binding:"required"`
		Query             string `json:"query" binding:"required"`
		MaxRows           int    `json:"maxRows"`
		TimeoutSeconds    int    `json:"timeoutSeconds"`
		DisallowMultiStmt bool   `json:"disallowMultiStatement"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperr.Validation("sessionId and query are required"))
		return
	}

	result, err := h.queryService.ExecuteQuery(c.Request.Context(), service.ExecuteOptions{
		AccountID:         accountID.String(),
		ActorID:           userID,
		SessionID:         req.SessionID,
		DatabaseID:        databaseID,
		Query:             req.Query,
		MaxRows:           req.MaxRows,
		TimeoutSeconds:    req.TimeoutSeconds,
		DisallowMultiStmt: req.DisallowMultiStmt,
	})
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}
