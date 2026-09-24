//go:build unit

package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openCodeGoUsagePermissionRepo struct {
	service.OperatorPermissionRepository
	service.AdminResourceScopeRepository
	scope service.AdminResourceScope
}

func (r *openCodeGoUsagePermissionRepo) GetAdminResourceScope(context.Context, int64) (service.AdminResourceScope, error) {
	return r.scope, nil
}

func TestOpenCodeGoUsageManagementRemainsSuperAdminOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tt := range []struct {
		name       string
		role       string
		scope      service.AdminResourceScope
		wantStatus int
	}{
		{"admin all accounts", service.RoleAdmin, service.UnrestrictedAdminResourceScope(), http.StatusForbidden},
		{"admin directly bound account", service.RoleAdmin, service.AdminResourceScope{Mode: service.AdminResourceScopeRestricted, AccountIDs: []int64{7}}, http.StatusForbidden},
		{"super admin reaches unavailable handler", service.RoleSuperAdmin, service.UnrestrictedAdminResourceScope(), http.StatusServiceUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			admin := router.Group("/api/v1/admin")
			admin.Use(gin.HandlerFunc(adminAuthForPermissionTest(tt.role, service.AdminPermissionAccountsWrite)))
			permissions := service.NewPermissionService(&openCodeGoUsagePermissionRepo{scope: tt.scope}, nil, nil)
			accountHandler := adminhandler.NewAccountHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, permissions)
			registerAccountRoutes(admin, &handler.Handlers{Admin: &handler.AdminHandlers{Account: accountHandler}}, servermiddleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() }))

			for _, request := range []struct {
				method string
				path   string
			}{
				{http.MethodGet, "/api/v1/admin/accounts/opencode-go-usage/settings"},
				{http.MethodPut, "/api/v1/admin/accounts/opencode-go-usage/settings"},
				{http.MethodGet, "/api/v1/admin/accounts/7/opencode-go-usage"},
				{http.MethodPut, "/api/v1/admin/accounts/7/opencode-go-usage/auto-refresh"},
				{http.MethodPost, "/api/v1/admin/accounts/7/opencode-go-usage/refresh"},
			} {
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, httptest.NewRequest(request.method, request.path, nil))
				require.Equal(t, tt.wantStatus, recorder.Code, "%s %s", request.method, request.path)
			}
		})
	}
}
