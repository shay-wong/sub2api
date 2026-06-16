package middleware

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func RequireAdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := GetUserRoleFromContext(c)
		if service.RoleIsAdmin(role) {
			c.Next()
			return
		}
		AbortWithError(c, http.StatusForbidden, "FORBIDDEN", "Admin access required")
	}
}

func RequireAdminPermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := GetUserRoleFromContext(c)
		if service.RoleIsAdmin(role) {
			c.Next()
			return
		}
		if service.RoleIsOperator(role) && operatorHasAdminPermission(permission) {
			c.Next()
			return
		}
		AbortWithError(c, http.StatusForbidden, "FORBIDDEN", "Insufficient admin permission")
	}
}

func operatorHasAdminPermission(permission string) bool {
	switch permission {
	case service.AdminPermissionDashboardRead,
		service.AdminPermissionOpsRead,
		service.AdminPermissionAccountsRead,
		service.AdminPermissionAccountsWrite:
		return true
	default:
		return false
	}
}
