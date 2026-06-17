package middleware

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func RequireAdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := GetUserRoleFromContext(c)
		if service.RoleIsSuperAdmin(role) {
			c.Next()
			return
		}
		AbortWithError(c, http.StatusForbidden, "FORBIDDEN", "Super admin access required")
	}
}

func RequireAdminPermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := GetUserRoleFromContext(c)
		if service.RoleIsAdmin(role) {
			c.Next()
			return
		}
		AbortWithError(c, http.StatusForbidden, "FORBIDDEN", "Insufficient admin permission")
	}
}
