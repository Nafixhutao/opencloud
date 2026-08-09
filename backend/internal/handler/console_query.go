package handler

import (
	"errors"
	"net/http"

	"github.com/Nafixhutao/opencloud/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// ConsoleQueryHandler handles SQL query execution operations
type ConsoleQueryHandler struct {
	queryService *service.ConsoleQueryService
}

// NewConsoleQueryHandler creates a new query handler
func NewConsoleQueryHandler(queryService *service.ConsoleQueryService) *ConsoleQueryHandler {
	return &ConsoleQueryHandler{queryService: queryService}
}

// ExecuteQuery executes a SQL query in the database console
// POST /api/v1/databases/:databaseId/console/execute
func (h *ConsoleQueryHandler) ExecuteQuery(c *gin.Context) {
	databaseID := c.Param("databaseId")
	accountID := c.GetString("account_id")

	var req struct {
		SessionID          string `json:"sessionId" binding:"required"`
		Query              string `json:"query" binding:"required"`
		MaxRows            int    `json:"maxRows"`
		TimeoutSeconds     int    `json:"timeoutSeconds"`
		DisallowMultiStmt  bool   `json:"disallowMultiStatement"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	result, err := h.queryService.ExecuteQuery(c.Request.Context(), service.ExecuteOptions{
		AccountID:         accountID,
		SessionID:         req.SessionID,
		DatabaseID:        databaseID,
		Query:             req.Query,
		MaxRows:           req.MaxRows,
		TimeoutSeconds:    req.TimeoutSeconds,
		DisallowMultiStmt: req.DisallowMultiStmt,
	})

	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": result,
	})
}

func handleError(c *gin.Context, err error) {
	var svcErr *service.ServiceError
	if errors.As(err, &svcErr) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": svcErr.Code, "message": svcErr.Message})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": err.Error()})
}
