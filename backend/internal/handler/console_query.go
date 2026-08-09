package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/middleware"
	"github.com/nazxf/opencloud/backend/internal/service"
)

// ConsoleQueryHandler handles SQL console query execution.
type ConsoleQueryHandler struct {
	svc *service.ConsoleQueryService
}

// NewConsoleQueryHandler constructs a ConsoleQueryHandler.
func NewConsoleQueryHandler(svc *service.ConsoleQueryService) *ConsoleQueryHandler {
	return &ConsoleQueryHandler{svc: svc}
}

// ConsoleQueryRequest represents the request body for query execution.
type ConsoleQueryRequest struct {
	SessionToken string `json:"session_token"`
	Query        string `json:"query" binding:"required,max=1048576"`
}

// ExecuteQuery handles POST /api/v1/databases/:id/console/query.
func (h *ConsoleQueryHandler) ExecuteQuery(c *gin.Context) {
	databaseID, ok := managedDatabaseIDParam(c)
	if !ok {
		return
	}

	var req ConsoleQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperr.Validation("invalid request body"))
		return
	}

	actorID := middleware.UserID(c)
	accountID := middleware.AccountID(c)

	result, err := h.svc.ExecuteQuery(
		c.Request.Context(),
		accountID,
		databaseID,
		req.SessionToken,
		req.Query,
	)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}
