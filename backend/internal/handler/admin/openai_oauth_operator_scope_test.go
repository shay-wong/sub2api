package admin

import (
	"bytes"
	"encoding/json"
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
	handler := NewOpenAIOAuthHandler(nil, adminSvc, permissionSvc)
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 101})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleOperator)
		c.Next()
	})
	router.POST("/api/v1/admin/openai/accounts/:id/refresh", handler.RefreshAccountToken)
	router.POST("/api/v1/admin/openai/generate-auth-url", handler.GenerateAuthURL)
	router.POST("/api/v1/admin/openai/create-from-oauth", handler.CreateAccountFromOAuth)
	return router
}

func TestOperatorOpenAIOAuthRefreshRejectsOutOfScopeAccount(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.accounts = []service.Account{{
		ID:          1,
		Name:        "hidden-openai",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		GroupIDs:    []int64{30},
		Credentials: map[string]any{"refresh_token": "rt"},
	}}
	router := newOperatorOpenAIOAuthScopeRouter(adminSvc, []int64{10})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/openai/accounts/1/refresh", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "OPERATOR_ACCOUNT_FORBIDDEN")
}

func TestOperatorOpenAIOAuthCreateRequiresAssignedGroups(t *testing.T) {
	adminSvc := newStubAdminService()
	router := newOperatorOpenAIOAuthScopeRouter(adminSvc, []int64{10})
	body, _ := json.Marshal(map[string]any{
		"session_id": "session",
		"code":       "code",
		"state":      "state",
		"group_ids":  []int64{},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/openai/create-from-oauth", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "OPERATOR_ACCOUNT_SCOPE_REQUIRED")
	require.Empty(t, adminSvc.createdAccounts)
}

func TestOperatorOpenAIOAuthCreateRejectsOutOfScopeGroups(t *testing.T) {
	adminSvc := newStubAdminService()
	router := newOperatorOpenAIOAuthScopeRouter(adminSvc, []int64{10})
	body, _ := json.Marshal(map[string]any{
		"session_id": "session",
		"code":       "code",
		"state":      "state",
		"group_ids":  []int64{30},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/openai/create-from-oauth", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "OPERATOR_SCOPE_FORBIDDEN")
	require.Empty(t, adminSvc.createdAccounts)
}

func TestOperatorOpenAIOAuthGenerateAuthURLRejectsProxyAssignment(t *testing.T) {
	adminSvc := newStubAdminService()
	router := newOperatorOpenAIOAuthScopeRouter(adminSvc, []int64{10})
	body, _ := json.Marshal(map[string]any{
		"proxy_id": 4,
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/openai/generate-auth-url", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "OPERATOR_PROXY_FORBIDDEN")
}

func TestOperatorOAuthProxyUseAllowsVisibleAccountCurrentProxy(t *testing.T) {
	adminSvc := newStubAdminService()
	proxyID := int64(4)
	accountID := int64(1)
	adminSvc.accounts = []service.Account{{
		ID:       1,
		Name:     "visible-openai",
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Status:   service.StatusActive,
		GroupIDs: []int64{10},
		ProxyID:  &proxyID,
	}}
	scope := &adminAccessScope{
		GroupIDs: []int64{10},
		groupSet: map[int64]struct{}{10: {}},
	}
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/openai/generate-auth-url", nil)

	err := scope.ensureOAuthProxyUse(c, adminSvc, &accountID, &proxyID)
	require.NoError(t, err)
}

func TestOperatorOpenAIOAuthGenerateAuthURLRejectsOutOfScopeAccountProxyReuse(t *testing.T) {
	adminSvc := newStubAdminService()
	proxyID := int64(4)
	adminSvc.accounts = []service.Account{{
		ID:       1,
		Name:     "hidden-openai",
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Status:   service.StatusActive,
		GroupIDs: []int64{30},
		ProxyID:  &proxyID,
	}}
	router := newOperatorOpenAIOAuthScopeRouter(adminSvc, []int64{10})
	body, _ := json.Marshal(map[string]any{
		"account_id": 1,
		"proxy_id":   4,
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/openai/generate-auth-url", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "OPERATOR_ACCOUNT_FORBIDDEN")
}

func TestOperatorOpenAIOAuthCreateRejectsProxyAssignment(t *testing.T) {
	adminSvc := newStubAdminService()
	router := newOperatorOpenAIOAuthScopeRouter(adminSvc, []int64{10})
	body, _ := json.Marshal(map[string]any{
		"session_id": "session",
		"code":       "code",
		"state":      "state",
		"group_ids":  []int64{10},
		"proxy_id":   4,
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/openai/create-from-oauth", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "OPERATOR_PROXY_FORBIDDEN")
	require.Empty(t, adminSvc.createdAccounts)
}
