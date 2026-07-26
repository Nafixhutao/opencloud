package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/middleware"
	"github.com/nazxf/opencloud/backend/internal/service"
)

// NodeHandler serves explicit global platform-admin node routes.
type NodeHandler struct {
	svc *service.NodeService
}

// NewNodeHandler constructs a NodeHandler.
func NewNodeHandler(svc *service.NodeService) *NodeHandler {
	return &NodeHandler{svc: svc}
}

// List handles GET /api/v1/admin/nodes.
func (h *NodeHandler) List(c *gin.Context) {
	nodes, err := h.svc.List(c.Request.Context(), middleware.UserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": nodes})
}

// Register handles POST /api/v1/admin/nodes.
func (h *NodeHandler) Register(c *gin.Context) {
	var req service.RegisterNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperr.Validation("invalid request body"))
		return
	}
	node, err := h.svc.Register(c.Request.Context(), middleware.UserID(c), req)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": node})
}

// SetStatus handles PATCH /api/v1/admin/nodes/:id.
func (h *NodeHandler) SetStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, apperr.Validation("invalid node id"))
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperr.Validation("invalid request body"))
		return
	}
	node, err := h.svc.SetStatus(c.Request.Context(), middleware.UserID(c), id, req.Status)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": node})
}
