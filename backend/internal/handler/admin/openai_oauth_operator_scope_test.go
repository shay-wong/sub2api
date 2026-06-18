package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newOperatorOpenAIOAuthScopeRouter(adminSvc *stubAdminService, groupIDs []int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	permissionSvc := service.NewPermissionService(
		&operatorPermissionRepoStub{scopes: map[int64][]int64{101: groupIDs}},
		operatorUserRepoStub{},
		operatorGroupRepoStub{},
	)
	handler := NewOpenAIOAuthHandler(nil, adminSvc, permissionSvc, nil)
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 101})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleOperator)
		c.Next()
	})
	router.POST("/api/v1/admin/openai/accounts/:id/refresh", handler.RefreshAccountToken)
	router.GET("/api/v1/admin/openai/accounts/:id/quota", handler.QueryQuota)
	router.POST("/api/v1/admin/openai/accounts/:id/reset-quota", handler.ResetQuota)
	router.POST("/api/v1/admin/openai/generate-auth-url", handler.GenerateAuthURL)
	router.POST("/api/v1/admin/openai/create-from-oauth", handler.CreateAccountFromOAuth)
	return router
}

func TestOperatorOpenAIOAuthRoutesRejectLegacyRole(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.accounts = []service.Account{{
		ID:          1,
		Name:        "visible-openai",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		GroupIDs:    []int64{10},
		Credentials: map[string]any{"refresh_token": "rt"},
	}}
	router := newOperatorOpenAIOAuthScopeRouter(adminSvc, []int64{10})

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "refresh", method: http.MethodPost, path: "/api/v1/admin/openai/accounts/1/refresh"},
		{name: "quota", method: http.MethodGet, path: "/api/v1/admin/openai/accounts/1/quota"},
		{name: "reset_quota", method: http.MethodPost, path: "/api/v1/admin/openai/accounts/1/reset-quota"},
		{name: "generate_auth_url", method: http.MethodPost, path: "/api/v1/admin/openai/generate-auth-url", body: `{"account_id":1,"proxy_id":4}`},
		{name: "create_from_oauth", method: http.MethodPost, path: "/api/v1/admin/openai/create-from-oauth", body: `{"session_id":"session","code":"code","state":"state","group_ids":[10],"proxy_id":4}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusForbidden, rec.Code)
			require.Contains(t, rec.Body.String(), "LEGACY_OPERATOR_ROLE_DISABLED")
		})
	}
	require.Empty(t, adminSvc.createdAccounts)
}
