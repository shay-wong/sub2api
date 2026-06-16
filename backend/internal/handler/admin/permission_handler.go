package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type PermissionHandler struct {
	permissionService *service.PermissionService
}

func NewPermissionHandler(permissionService *service.PermissionService) *PermissionHandler {
	return &PermissionHandler{permissionService: permissionService}
}

type updateOperatorPermissionRequest struct {
	Role     string  `json:"role" binding:"required"`
	GroupIDs []int64 `json:"group_ids"`
}

func (h *PermissionHandler) ListOperators(c *gin.Context) {
	if h.permissionService == nil {
		response.InternalError(c, "Permission service unavailable")
		return
	}
	subjects, err := h.permissionService.ListOperatorPermissions(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, subjects)
}

func (h *PermissionHandler) UpdateOperator(c *gin.Context) {
	if h.permissionService == nil {
		response.InternalError(c, "Permission service unavailable")
		return
	}
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	var req updateOperatorPermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	var createdBy *int64
	if subject, ok := middleware.GetAuthSubjectFromContext(c); ok && subject.UserID > 0 {
		createdBy = &subject.UserID
	}
	updated, err := h.permissionService.SetOperatorPermissions(c.Request.Context(), userID, req.Role, req.GroupIDs, createdBy)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, updated)
}
