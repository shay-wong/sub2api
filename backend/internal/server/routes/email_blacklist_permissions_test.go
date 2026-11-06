//go:build unit

package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestEmailBlacklistRoutesPreserveAdminPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name        string
		role        string
		permissions []string
		wantUsers   int
		wantSetting int
	}{
		{name: "admin_without_users_permission", role: service.RoleAdmin, wantUsers: http.StatusForbidden, wantSetting: http.StatusForbidden},
		{name: "admin_with_users_permission", role: service.RoleAdmin, permissions: []string{service.AdminPermissionUsersManage}, wantUsers: http.StatusBadRequest, wantSetting: http.StatusForbidden},
		{name: "super_admin", role: service.RoleSuperAdmin, wantUsers: http.StatusBadRequest, wantSetting: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			auth := middleware.AdminAuthMiddleware(func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyUserRole), tc.role)
				c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), tc.permissions))
				c.Next()
			})
			handlers := &handler.Handlers{Admin: &handler.AdminHandlers{
				User:    adminhandler.NewUserHandler(nil, nil, nil, nil, nil, nil, nil),
				Setting: adminhandler.NewSettingHandler(nil, nil, nil, nil, nil, nil, nil),
			}}
			RegisterAdminRoutes(router.Group("/api/v1"), handlers, auth,
				middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() }),
				middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() }), nil, nil)

			for _, endpoint := range []struct {
				method string
				path   string
				status int
			}{
				{http.MethodPost, "/api/v1/admin/users/blacklist-matches", tc.wantUsers},
				{http.MethodPost, "/api/v1/admin/users/batch-disable", tc.wantUsers},
				{http.MethodPut, "/api/v1/admin/settings/registration-email-blacklist", tc.wantSetting},
			} {
				// Malformed input yields 400 only after the route's authorization has succeeded.
				request := httptest.NewRequest(endpoint.method, endpoint.path, strings.NewReader("["))
				request.Header.Set("Content-Type", "application/json")
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, request)
				require.Equal(t, endpoint.status, recorder.Code, endpoint.path+": "+recorder.Body.String())
			}
		})
	}
}

func TestEmailBlacklistRoutesDenyRestrictedAdminKeys(t *testing.T) {
	policy := adminaccess.Policy{GlobalPermissions: []string{"users.read", "users.quotas.write"}}
	for _, endpoint := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/admin/settings/registration-email-blacklist"},
		{http.MethodPut, "/api/v1/admin/settings/registration-email-blacklist"},
		{http.MethodPost, "/api/v1/admin/users/blacklist-matches"},
		{http.MethodPost, "/api/v1/admin/users/batch-disable"},
	} {
		require.False(t, adminaccess.RouteAllowed(policy, endpoint.method, endpoint.path), endpoint.path)
	}
}
