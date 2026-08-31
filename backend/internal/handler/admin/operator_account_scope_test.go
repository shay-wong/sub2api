package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type operatorPermissionRepoStub struct {
	scopes      map[int64][]int64
	adminScopes map[int64]service.AdminResourceScope
}

func (r *operatorPermissionRepoStub) ListOperatorPermissionSubjects(context.Context) ([]service.OperatorPermissionSubject, error) {
	return nil, nil
}

func (r *operatorPermissionRepoStub) GetOperatorGroupIDs(_ context.Context, userID int64) ([]int64, error) {
	return append([]int64(nil), r.scopes[userID]...), nil
}

func (r *operatorPermissionRepoStub) GetOperatorScopesByUserIDs(context.Context, []int64) (map[int64][]int64, error) {
	return nil, nil
}

func (r *operatorPermissionRepoStub) SetOperatorGroupIDs(context.Context, int64, []int64, *int64) error {
	return nil
}

func (r *operatorPermissionRepoStub) ClearOperatorGroupIDs(context.Context, int64) error {
	return nil
}

func (r *operatorPermissionRepoStub) GetAdminResourceScope(_ context.Context, userID int64) (service.AdminResourceScope, error) {
	if scope, ok := r.adminScopes[userID]; ok {
		return scope, nil
	}
	return service.UnrestrictedAdminResourceScope(), nil
}

func (r *operatorPermissionRepoStub) GetAdminResourceScopesByUserIDs(_ context.Context, userIDs []int64) (map[int64]service.AdminResourceScope, error) {
	out := make(map[int64]service.AdminResourceScope, len(userIDs))
	for _, userID := range userIDs {
		scope, err := r.GetAdminResourceScope(context.Background(), userID)
		if err != nil {
			return nil, err
		}
		out[userID] = scope
	}
	return out, nil
}

func (r *operatorPermissionRepoStub) UpdateUserAdminAccess(context.Context, int64, string, []string, service.AdminResourceScope, *int64) error {
	return nil
}

func (r *operatorPermissionRepoStub) BindAdminResource(context.Context, int64, string, int64, *int64) error {
	return nil
}

type operatorUserRepoStub struct{}

func (operatorUserRepoStub) Create(context.Context, *service.User) error { return nil }
func (operatorUserRepoStub) CreateWithEmailAliasGuard(context.Context, *service.User) error {
	return nil
}
func (operatorUserRepoStub) GetByID(context.Context, int64) (*service.User, error) {
	return &service.User{ID: 101, Role: service.RoleOperator, Status: service.StatusActive}, nil
}
func (operatorUserRepoStub) GetByIDIncludeDeleted(context.Context, int64) (*service.User, error) {
	return nil, nil
}
func (operatorUserRepoStub) GetByEmail(context.Context, string) (*service.User, error) {
	return nil, nil
}
func (operatorUserRepoStub) GetFirstAdmin(context.Context) (*service.User, error) { return nil, nil }
func (operatorUserRepoStub) IncrementTokenVersion(context.Context, int64) error   { return nil }
func (operatorUserRepoStub) Update(context.Context, *service.User, service.UserUpdateFields) error {
	return nil
}
func (operatorUserRepoStub) Delete(context.Context, int64) error { return nil }
func (operatorUserRepoStub) BatchUpdateLimits(context.Context, []int64, *int, *int) (int, error) {
	return 0, nil
}
func (operatorUserRepoStub) GetUserAvatar(context.Context, int64) (*service.UserAvatar, error) {
	return nil, nil
}
func (operatorUserRepoStub) UpsertUserAvatar(context.Context, int64, service.UpsertUserAvatarInput) (*service.UserAvatar, error) {
	return nil, nil
}
func (operatorUserRepoStub) DeleteUserAvatar(context.Context, int64) error { return nil }
func (operatorUserRepoStub) List(context.Context, pagination.PaginationParams) ([]service.User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (operatorUserRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, service.UserListFilters) ([]service.User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (operatorUserRepoStub) GetLatestUsedAtByUserIDs(context.Context, []int64) (map[int64]*time.Time, error) {
	return nil, nil
}
func (operatorUserRepoStub) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	return nil, nil
}
func (operatorUserRepoStub) UpdateUserLastActiveAt(context.Context, int64, time.Time) error {
	return nil
}
func (operatorUserRepoStub) UpdateBalance(context.Context, int64, float64) error { return nil }
func (operatorUserRepoStub) AdjustBalance(context.Context, int64, float64) (service.BalanceChange, error) {
	return service.BalanceChange{}, nil
}
func (operatorUserRepoStub) SetBalance(context.Context, int64, float64) (service.BalanceChange, error) {
	return service.BalanceChange{}, nil
}
func (operatorUserRepoStub) DeductBalance(context.Context, int64, float64) error { return nil }
func (operatorUserRepoStub) UpdateConcurrency(context.Context, int64, int) error { return nil }
func (operatorUserRepoStub) BatchSetConcurrency(context.Context, []int64, int) (int, error) {
	return 0, nil
}
func (operatorUserRepoStub) BatchAddConcurrency(context.Context, []int64, int) (int, error) {
	return 0, nil
}
func (operatorUserRepoStub) ExistsByEmail(context.Context, string) (bool, error) { return false, nil }
func (operatorUserRepoStub) ExistsByEmailAlias(context.Context, string) (bool, error) {
	return false, nil
}
func (operatorUserRepoStub) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	return 0, nil
}
func (operatorUserRepoStub) AddGroupToAllowedGroups(context.Context, int64, int64) error {
	return nil
}
func (operatorUserRepoStub) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	return nil
}
func (operatorUserRepoStub) ListUserAuthIdentities(context.Context, int64) ([]service.UserAuthIdentityRecord, error) {
	return nil, nil
}
func (operatorUserRepoStub) UnbindUserAuthProvider(context.Context, int64, string) error {
	return nil
}
func (operatorUserRepoStub) UpdateTotpSecret(context.Context, int64, *string) error { return nil }
func (operatorUserRepoStub) EnableTotp(context.Context, int64) error                { return nil }
func (operatorUserRepoStub) DisableTotp(context.Context, int64) error               { return nil }

type operatorGroupRepoStub struct{}

func (operatorGroupRepoStub) Create(context.Context, *service.Group) error { return nil }
func (operatorGroupRepoStub) GetByID(_ context.Context, id int64) (*service.Group, error) {
	return &service.Group{ID: id, Name: "group", Status: service.StatusActive}, nil
}
func (operatorGroupRepoStub) GetByIDLite(ctx context.Context, id int64) (*service.Group, error) {
	return operatorGroupRepoStub{}.GetByID(ctx, id)
}
func (operatorGroupRepoStub) Update(context.Context, *service.Group) error { return nil }
func (operatorGroupRepoStub) Delete(context.Context, int64) error          { return nil }
func (operatorGroupRepoStub) DeleteCascade(context.Context, int64) ([]int64, error) {
	return nil, nil
}
func (operatorGroupRepoStub) List(context.Context, pagination.PaginationParams) ([]service.Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (operatorGroupRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, *bool) ([]service.Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (operatorGroupRepoStub) ListActive(context.Context) ([]service.Group, error) { return nil, nil }
func (operatorGroupRepoStub) ListActiveByPlatform(context.Context, string) ([]service.Group, error) {
	return nil, nil
}
func (operatorGroupRepoStub) ExistsByName(context.Context, string) (bool, error) { return false, nil }
func (operatorGroupRepoStub) GetAccountCount(context.Context, int64) (int64, int64, error) {
	return 0, 0, nil
}
func (operatorGroupRepoStub) DeleteAccountGroupsByGroupID(context.Context, int64) (int64, error) {
	return 0, nil
}
func (operatorGroupRepoStub) GetAccountIDsByGroupIDs(context.Context, []int64) ([]int64, error) {
	return nil, nil
}
func (operatorGroupRepoStub) BindAccountsToGroup(context.Context, int64, []int64) error {
	return nil
}
func (operatorGroupRepoStub) UpdateSortOrders(context.Context, []service.GroupSortOrderUpdate) error {
	return nil
}

func newOperatorAccountScopeRouter(adminSvc *stubAdminService, groupIDs []int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	permissionSvc := service.NewPermissionService(
		&operatorPermissionRepoStub{scopes: map[int64][]int64{101: groupIDs}},
		operatorUserRepoStub{},
		operatorGroupRepoStub{},
	)
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, permissionSvc)
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 101})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleOperator)
		c.Next()
	})
	router.GET("/api/v1/admin/accounts", handler.List)
	router.GET("/api/v1/admin/accounts/:id", handler.GetByID)
	router.POST("/api/v1/admin/accounts", handler.Create)
	router.PUT("/api/v1/admin/accounts/:id", handler.Update)
	router.POST("/api/v1/admin/accounts/bulk-update", handler.BulkUpdate)
	return router
}

func newAdminAccountScopeRouter(adminSvc *stubAdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 101})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleAdmin)
		c.Request = c.Request.WithContext(service.WithAdminRole(c.Request.Context(), service.RoleAdmin))
		c.Next()
	})
	router.GET("/api/v1/admin/accounts", handler.List)
	router.GET("/api/v1/admin/accounts/:id", handler.GetByID)
	router.POST("/api/v1/admin/accounts", handler.Create)
	router.PUT("/api/v1/admin/accounts/:id", handler.Update)
	router.POST("/api/v1/admin/accounts/bulk-update", handler.BulkUpdate)
	router.POST("/api/v1/admin/accounts/:id/apply-oauth-credentials", handler.ApplyOAuthCredentials)
	return router
}

func newRestrictedAdminAccountScopeRouter(adminSvc *stubAdminService, scope service.AdminResourceScope) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	permissionSvc := service.NewPermissionService(
		&operatorPermissionRepoStub{adminScopes: map[int64]service.AdminResourceScope{101: scope}},
		operatorUserRepoStub{},
		operatorGroupRepoStub{},
	)
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, permissionSvc)
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 101})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleAdmin)
		c.Request = c.Request.WithContext(service.WithAdminRole(c.Request.Context(), service.RoleAdmin))
		c.Next()
	})
	router.GET("/api/v1/admin/accounts", handler.List)
	router.GET("/api/v1/admin/accounts/:id", handler.GetByID)
	router.GET("/api/v1/admin/accounts/proxy-options", handler.GetProxyOptions)
	router.POST("/api/v1/admin/accounts/batch-delete", handler.BatchDelete)
	router.POST("/api/v1/admin/accounts/bulk-update", handler.BulkUpdate)
	return router
}

func TestRestrictedAdminAccountScopeUsesDirectBindingsOnly(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.accounts = []service.Account{
		{ID: 1, Name: "group-only", Status: service.StatusActive, GroupIDs: []int64{10}},
		{ID: 2, Name: "direct", Status: service.StatusActive, GroupIDs: []int64{20}},
	}
	router := newRestrictedAdminAccountScopeRouter(adminSvc, service.AdminResourceScope{
		Mode:       service.AdminResourceScopeRestricted,
		GroupIDs:   []int64{10},
		AccountIDs: []int64{2},
	})

	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts?page=1&page_size=20", nil))
	require.Equal(t, http.StatusOK, listRec.Code)
	require.NotContains(t, listRec.Body.String(), "group-only")
	require.Contains(t, listRec.Body.String(), "direct")

	groupOnlyRec := httptest.NewRecorder()
	router.ServeHTTP(groupOnlyRec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/1", nil))
	require.Equal(t, http.StatusForbidden, groupOnlyRec.Code)
	require.Contains(t, groupOnlyRec.Body.String(), "ADMIN_ACCOUNT_SCOPE_FORBIDDEN")

	directRec := httptest.NewRecorder()
	router.ServeHTTP(directRec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/2", nil))
	require.Equal(t, http.StatusOK, directRec.Code)
}

func TestRestrictedAdminAccountBatchOperationsUseDirectBindings(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.accounts = []service.Account{
		{ID: 1, Name: "group-only", Status: service.StatusActive, GroupIDs: []int64{10}},
		{ID: 2, Name: "direct", Status: service.StatusActive, GroupIDs: []int64{10}},
	}
	router := newRestrictedAdminAccountScopeRouter(adminSvc, service.AdminResourceScope{
		Mode:       service.AdminResourceScopeRestricted,
		GroupIDs:   []int64{10},
		AccountIDs: []int64{2},
	})

	deleteRec := httptest.NewRecorder()
	deleteReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/batch-delete", bytes.NewBufferString(`{"account_ids":[1]}`))
	deleteReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(deleteRec, deleteReq)
	require.Equal(t, http.StatusForbidden, deleteRec.Code)
	require.Contains(t, deleteRec.Body.String(), "ADMIN_ACCOUNT_SCOPE_FORBIDDEN")

	bulkRec := httptest.NewRecorder()
	bulkReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/bulk-update", bytes.NewBufferString(`{"filters":{"group":"10"},"schedulable":true}`))
	bulkReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(bulkRec, bulkReq)
	require.Equal(t, http.StatusOK, bulkRec.Code)
	require.NotNil(t, adminSvc.lastBulkUpdateAccountInput)
	require.Equal(t, []int64{2}, adminSvc.lastBulkUpdateAccountInput.AccountScopeIDs)
}

func TestRestrictedAdminProxyCollectionsUseDirectBindings(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.proxies = []service.Proxy{
		{ID: 10, Name: "hidden", Status: service.StatusActive},
		{ID: 20, Name: "visible", Status: service.StatusActive},
	}
	scope := service.AdminResourceScope{
		Mode:       service.AdminResourceScopeRestricted,
		AccountIDs: []int64{2},
		ProxyIDs:   []int64{20},
	}
	permissionSvc := service.NewPermissionService(
		&operatorPermissionRepoStub{adminScopes: map[int64]service.AdminResourceScope{101: scope}},
		operatorUserRepoStub{},
		operatorGroupRepoStub{},
	)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 101})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleAdmin)
		c.Next()
	})
	accountHandler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, permissionSvc)
	proxyHandler := NewProxyHandler(&scopedProxyAccountsAdminService{
		stubAdminService: adminSvc,
		proxyAccounts: []service.ProxyAccountSummary{
			{ID: 1, Name: "hidden-account"},
			{ID: 2, Name: "visible-account"},
		},
	}, permissionSvc)
	router.GET("/api/v1/admin/accounts/proxy-options", accountHandler.GetProxyOptions)
	router.GET("/api/v1/admin/proxies/:id/accounts", proxyHandler.GetProxyAccounts)

	optionsRec := httptest.NewRecorder()
	router.ServeHTTP(optionsRec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/proxy-options", nil))
	require.Equal(t, http.StatusOK, optionsRec.Code)
	require.NotContains(t, optionsRec.Body.String(), "hidden")
	require.Contains(t, optionsRec.Body.String(), "visible")

	accountsRec := httptest.NewRecorder()
	router.ServeHTTP(accountsRec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/proxies/20/accounts", nil))
	require.Equal(t, http.StatusOK, accountsRec.Code)
	require.NotContains(t, accountsRec.Body.String(), "hidden-account")
	require.Contains(t, accountsRec.Body.String(), "visible-account")
}

type scopedProxyAccountsAdminService struct {
	*stubAdminService
	proxyAccounts []service.ProxyAccountSummary
}

func (s *scopedProxyAccountsAdminService) GetProxyAccounts(context.Context, int64) ([]service.ProxyAccountSummary, error) {
	return s.proxyAccounts, nil
}

func TestOperatorAccountRoutesRejectLegacyRole(t *testing.T) {
	adminSvc := newStubAdminService()
	router := newOperatorAccountScopeRouter(adminSvc, []int64{10})
	adminSvc.accounts = []service.Account{{ID: 1, Name: "visible", Status: service.StatusActive, GroupIDs: []int64{10}}}

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "list", method: http.MethodGet, path: "/api/v1/admin/accounts?page=1&page_size=20"},
		{name: "detail", method: http.MethodGet, path: "/api/v1/admin/accounts/1"},
		{name: "create", method: http.MethodPost, path: "/api/v1/admin/accounts", body: `{"name":"operator-created","platform":"anthropic","type":"apikey","credentials":{"api_key":"sk-test"},"group_ids":[10]}`},
		{name: "update", method: http.MethodPut, path: "/api/v1/admin/accounts/1", body: `{"name":"operator-updated"}`},
		{name: "bulk_update", method: http.MethodPost, path: "/api/v1/admin/accounts/bulk-update", body: `{"filters":{"platform":"openai"},"schedulable":true}`},
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
	require.Nil(t, adminSvc.lastBulkUpdateInput)
}

func TestAdminAccountRoutesProtectManagedUpstreamBillingProbeState(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.accounts = []service.Account{{
		ID:       1,
		Name:     "project-account",
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeAPIKey,
		Status:   service.StatusActive,
		Extra: map[string]any{
			"visible": "kept",
			service.UpstreamBillingProbeEnabledExtraKey:    true,
			service.UpstreamBillingRateSyncEnabledExtraKey: true,
			service.UpstreamBillingProbeExtraKey:           map[string]any{"status": "ok"},
		},
	}}
	router := newAdminAccountScopeRouter(adminSvc)

	for _, path := range []string{
		"/api/v1/admin/accounts?page=1&page_size=20",
		"/api/v1/admin/accounts/1",
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), `"visible":"kept"`)
		require.NotContains(t, rec.Body.String(), service.UpstreamBillingProbeEnabledExtraKey)
		require.NotContains(t, rec.Body.String(), service.UpstreamBillingRateSyncEnabledExtraKey)
		require.NotContains(t, rec.Body.String(), service.UpstreamBillingProbeExtraKey)
	}

	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "create typed probe", method: http.MethodPost, path: "/api/v1/admin/accounts", body: `{"name":"project-created","platform":"openai","type":"apikey","credentials":{"api_key":"sk-test"},"upstream_billing_probe_enabled":false}`},
		{name: "update typed probe", method: http.MethodPut, path: "/api/v1/admin/accounts/1", body: `{"upstream_billing_probe_enabled":false}`},
		{name: "update typed rate sync", method: http.MethodPut, path: "/api/v1/admin/accounts/1", body: `{"upstream_billing_rate_sync_enabled":true}`},
		{name: "update extra rate sync", method: http.MethodPut, path: "/api/v1/admin/accounts/1", body: `{"extra":{"upstream_billing_rate_sync_enabled":true}}`},
		{name: "bulk typed probe", method: http.MethodPost, path: "/api/v1/admin/accounts/bulk-update", body: `{"account_ids":[1],"upstream_billing_probe_enabled":false}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusForbidden, rec.Code)
			require.Contains(t, rec.Body.String(), "UPSTREAM_BILLING_PROBE_FORBIDDEN")
		})
	}
	require.Empty(t, adminSvc.createdAccounts)
	require.Zero(t, adminSvc.updateAccountCalls)
	require.Nil(t, adminSvc.lastBulkUpdateInput)

	adminSvc.accounts[0].Type = service.AccountTypeOAuth
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/apply-oauth-credentials", bytes.NewBufferString(`{
		"type": "oauth",
		"credentials": {"access_token": "new-token"},
		"extra": {"upstream_billing_probe_enabled": true}
	}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "UPSTREAM_BILLING_PROBE_FORBIDDEN")
	require.Zero(t, adminSvc.updateAccountCalls)
	require.Zero(t, adminSvc.updateAccountExtraCalls)
}
