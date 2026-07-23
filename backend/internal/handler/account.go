// Package handler holds Gin HTTP handlers (BACKEND.md §5).
package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/middleware"
	"github.com/nazxf/opencloud/backend/internal/service"
)

// AccountHandler serves /api/v1/me and admin user routes.
type AccountHandler struct {
	svc *service.AccountService
}

// NewAccountHandler constructs an AccountHandler.
func NewAccountHandler(svc *service.AccountService) *AccountHandler {
	return &AccountHandler{svc: svc}
}

// Me handles GET /api/v1/me.
func (h *AccountHandler) Me(c *gin.Context) {
	userID := middleware.UserID(c)
	// Display name is optional; used only when ensuring a legacy membership.
	displayName := c.GetHeader("X-User-Name")
	me, err := h.svc.GetMe(c.Request.Context(), userID, displayName)
	if err != nil {
		respondError(c, err)
		return
	}
	// Cross-check JWT account claim against server-side membership.
	if me.AccountID != middleware.AccountID(c) {
		respondError(c, apperr.Unauthenticated("token account mismatch"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": me})
}

// UpdateMe handles PATCH /api/v1/me.
func (h *AccountHandler) UpdateMe(c *gin.Context) {
	var req service.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperr.Validation("invalid request body"))
		return
	}
	me, err := h.svc.UpdateProfile(
		c.Request.Context(),
		middleware.UserID(c),
		middleware.AccountID(c),
		req,
	)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": me})
}

// ListUsers handles GET /api/v1/admin/users.
func (h *AccountHandler) ListUsers(c *gin.Context) {
	page := queryInt(c, "page", 1)
	perPage := queryInt(c, "per_page", 25)
	users, total, err := h.svc.ListUsers(c.Request.Context(), page, perPage)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": users,
		"meta": gin.H{"page": page, "per_page": perPage, "total": total},
	})
}

// GetUser handles GET /api/v1/admin/users/:id.
func (h *AccountHandler) GetUser(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, apperr.Validation("invalid user id"))
		return
	}
	user, err := h.svc.GetUser(c.Request.Context(), id)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": user})
}

// UpdateUser handles PATCH /api/v1/admin/users/:id.
func (h *AccountHandler) UpdateUser(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, apperr.Validation("invalid user id"))
		return
	}
	var req service.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperr.Validation("invalid request body"))
		return
	}
	user, err := h.svc.UpdateUser(c.Request.Context(), middleware.UserID(c), id, req)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": user})
}

func queryInt(c *gin.Context, key string, def int) int {
	v := c.Query(key)
	if v == "" {
		return def
	}
	var n int
	for _, ch := range v {
		if ch < '0' || ch > '9' {
			return def
		}
		n = n*10 + int(ch-'0')
	}
	if n == 0 {
		return def
	}
	return n
}

// respondError maps typed errors to the API envelope.
func respondError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	var ae *apperr.Error
	if errors.As(err, &ae) && ae != nil {
		body := gin.H{
			"error": gin.H{
				"code":    ae.Code,
				"message": ae.Message,
			},
		}
		if len(ae.Details) > 0 {
			body["error"].(gin.H)["details"] = ae.Details
		}
		c.AbortWithStatusJSON(ae.Status, body)
		return
	}
	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
		"error": gin.H{"code": "INTERNAL", "message": "internal server error"},
	})
}
