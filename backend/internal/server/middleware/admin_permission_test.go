//go:build unit

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequireAdminPermissionRoleHierarchy(t *testing.T) {
	tests := []struct {
		name       string
		role       string
		permission string
		wantStatus int
	}{
		{
			name:       "admin_allows_any_permission",
			role:       service.RoleAdmin,
			permission: service.AdminPermissionPermissionsWrite,
			wantStatus: http.StatusOK,
		},
		{
			name:       "operator_allows_accounts_write",
			role:       service.RoleOperator,
			permission: service.AdminPermissionAccountsWrite,
			wantStatus: http.StatusOK,
		},
		{
			name:       "operator_rejects_permissions_write",
			role:       service.RoleOperator,
			permission: service.AdminPermissionPermissionsWrite,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "user_rejects_accounts_write",
			role:       service.RoleUser,
			permission: service.AdminPermissionAccountsWrite,
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)

			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Set(string(ContextKeyUserRole), tc.role)
				c.Next()
			})
			r.Use(RequireAdminPermission(tc.permission))
			r.GET("/test", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"ok": true})
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			require.Equal(t, tc.wantStatus, w.Code)
		})
	}
}
