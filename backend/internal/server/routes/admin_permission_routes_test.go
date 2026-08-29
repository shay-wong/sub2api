//go:build unit

package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func registerAdminRoutesForContractTest(t *testing.T) []gin.RouteInfo {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	passthrough := func(c *gin.Context) { c.Next() }
	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{}},
		servermiddleware.AdminAuthMiddleware(passthrough),
		servermiddleware.AuditLogMiddleware(passthrough),
		servermiddleware.StepUpAuthMiddleware(passthrough),
		nil,
		nil,
	)
	return router.Routes()
}

func registerAdminRoutesForPermissionTest(
	router *gin.RouterGroup,
	handlers *handler.Handlers,
	auth servermiddleware.AdminAuthMiddleware,
	settingService *service.SettingService,
) {
	passthrough := func(c *gin.Context) { c.Next() }
	RegisterAdminRoutes(
		router,
		handlers,
		auth,
		servermiddleware.AuditLogMiddleware(passthrough),
		servermiddleware.StepUpAuthMiddleware(passthrough),
		settingService,
		nil,
	)
}

func registerPaymentRoutesForPermissionTest(
	router *gin.RouterGroup,
	paymentHandler *handler.PaymentHandler,
	webhookHandler *handler.PaymentWebhookHandler,
	adminPaymentHandler *adminhandler.PaymentHandler,
	jwtAuth servermiddleware.JWTAuthMiddleware,
	adminAuth servermiddleware.AdminAuthMiddleware,
	settingService *service.SettingService,
) {
	RegisterPaymentRoutes(
		router,
		paymentHandler,
		webhookHandler,
		adminPaymentHandler,
		jwtAuth,
		adminAuth,
		servermiddleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() }),
		settingService,
		nil,
	)
}

func adminAuthForPermissionTest(role string, permissions ...string) servermiddleware.AdminAuthMiddleware {
	return servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
		c.Set(string(servermiddleware.ContextKeyUserRole), role)
		c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), permissions))
		c.Next()
	})
}

func TestAdminRoutesDoNotExposeProjectSpace(t *testing.T) {
	for _, route := range registerAdminRoutesForContractTest(t) {
		require.False(t, strings.HasPrefix(route.Path, "/api/v1/admin/projects"), "%s %s", route.Method, route.Path)
	}
}

func TestAdminRoutesExposeIndependentAdminAccessEndpoint(t *testing.T) {
	found := false
	for _, route := range registerAdminRoutesForContractTest(t) {
		if route.Method == http.MethodPut && route.Path == "/api/v1/admin/users/:id/admin-access" {
			found = true
			break
		}
	}
	require.True(t, found)
}

func TestAdminAccessEndpointRequiresSuperAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	registerAdminRoutesForPermissionTest(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{User: &adminhandler.UserHandler{}}},
		adminAuthForPermissionTest(service.RoleAdmin, service.AdminPermissionUsersManage),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/42/admin-access", strings.NewReader(`{"role":"admin","admin_permissions":[]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestPaymentAdminRoutesRequireSuperAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	registerPaymentRoutesForPermissionTest(
		v1,
		&handler.PaymentHandler{},
		&handler.PaymentWebhookHandler{},
		&adminhandler.PaymentHandler{},
		servermiddleware.JWTAuthMiddleware(func(c *gin.Context) { c.Next() }),
		adminAuthForPermissionTest(service.RoleAdmin, service.AdminPermissionSubsManage),
		nil,
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/payment/config", nil))
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestAdminAPIKeyRoutesEnforceAccountsPermission(t *testing.T) {
	tests := []struct {
		name        string
		permissions []string
		wantStatus  int
		wantUpdate  bool
	}{
		{name: "allowed", permissions: []string{service.AdminPermissionAccountsWrite}, wantStatus: http.StatusOK, wantUpdate: true},
		{name: "forbidden", permissions: []string{service.AdminPermissionDashboardRead}, wantStatus: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			v1 := router.Group("/api/v1")
			adminSvc := &apiKeyRouteAdminServiceStub{}
			registerAdminRoutesForPermissionTest(
				v1,
				&handler.Handlers{Admin: &handler.AdminHandlers{APIKey: adminhandler.NewAdminAPIKeyHandler(adminSvc)}},
				adminAuthForPermissionTest(service.RoleAdmin, tt.permissions...),
				nil,
			)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", strings.NewReader(`{"group_id":2}`))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantUpdate {
				require.Equal(t, int64(10), adminSvc.updatedKeyID)
				require.NotNil(t, adminSvc.updatedGroupID)
				require.Equal(t, int64(2), *adminSvc.updatedGroupID)
			} else {
				require.Zero(t, adminSvc.updatedKeyID)
			}
		})
	}
}

func TestAdminGrokRuntimeSanityRequiresOpsPermission(t *testing.T) {
	tests := []struct {
		name        string
		permissions []string
		wantStatus  int
	}{
		{name: "allowed", permissions: []string{service.AdminPermissionOpsRead}, wantStatus: http.StatusOK},
		{name: "forbidden", permissions: []string{service.AdminPermissionDashboardRead}, wantStatus: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			v1 := router.Group("/api/v1")
			registerAdminRoutesForPermissionTest(
				v1,
				&handler.Handlers{Admin: &handler.AdminHandlers{GrokOAuth: adminhandler.NewGrokOAuthHandler(nil, nil, nil, nil, nil)}},
				adminAuthForPermissionTest(service.RoleAdmin, tt.permissions...),
				nil,
			)

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/grok/runtime-sanity", nil))
			require.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantStatus == http.StatusOK {
				require.Contains(t, rec.Body.String(), "base_url")
			}
		})
	}
}

func TestUsageRecordRuntimeRemainsSuperAdminOnly(t *testing.T) {
	tests := []struct {
		name       string
		role       string
		wantStatus int
	}{
		{name: "admin", role: service.RoleAdmin, wantStatus: http.StatusForbidden},
		{name: "super_admin", role: service.RoleSuperAdmin, wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			v1 := router.Group("/api/v1")
			opsSvc := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			registerAdminRoutesForPermissionTest(
				v1,
				&handler.Handlers{Admin: &handler.AdminHandlers{Ops: adminhandler.NewOpsHandler(opsSvc)}},
				adminAuthForPermissionTest(tt.role, service.AdminPermissionOpsRead),
				nil,
			)

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/runtime/usage-record", nil))
			require.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantStatus == http.StatusOK {
				require.Contains(t, rec.Body.String(), `"scope":"process"`)
			}
		})
	}
}

func TestAdminProxyRoutesAreGlobalButProtectSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	adminSvc := newProxyRouteAdminServiceStub()
	registerAdminRoutesForPermissionTest(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{Proxy: adminhandler.NewProxyHandler(adminSvc)}},
		adminAuthForPermissionTest(service.RoleAdmin, service.AdminPermissionProxiesManage),
		nil,
	)

	for _, path := range []string{
		"/api/v1/admin/proxies",
		"/api/v1/admin/proxies/all",
		"/api/v1/admin/proxies/all?with_count=true",
		"/api/v1/admin/proxies/10",
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, rec.Code, path)
		require.NotContains(t, rec.Body.String(), `"password"`, path)
		require.NotContains(t, rec.Body.String(), "secret-pass", path)
	}
	require.Positive(t, adminSvc.listCalls)
}

func TestAdminProxyRoutesRejectMissingPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	adminSvc := newProxyRouteAdminServiceStub()
	registerAdminRoutesForPermissionTest(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{Proxy: adminhandler.NewProxyHandler(adminSvc)}},
		adminAuthForPermissionTest(service.RoleAdmin, service.AdminPermissionDashboardRead),
		nil,
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/proxies", nil))
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Zero(t, adminSvc.listCalls)
}

func TestProxyExportAndRawSecretsRemainSuperAdminOnly(t *testing.T) {
	tests := []struct {
		name       string
		role       string
		path       string
		wantStatus int
		wantSecret bool
	}{
		{name: "admin export", role: service.RoleAdmin, path: "/api/v1/admin/proxies/data", wantStatus: http.StatusForbidden},
		{name: "super admin export", role: service.RoleSuperAdmin, path: "/api/v1/admin/proxies/data", wantStatus: http.StatusOK, wantSecret: true},
		{name: "super admin detail", role: service.RoleSuperAdmin, path: "/api/v1/admin/proxies/10", wantStatus: http.StatusOK, wantSecret: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			v1 := router.Group("/api/v1")
			adminSvc := newProxyRouteAdminServiceStub()
			registerAdminRoutesForPermissionTest(
				v1,
				&handler.Handlers{Admin: &handler.AdminHandlers{Proxy: adminhandler.NewProxyHandler(adminSvc)}},
				adminAuthForPermissionTest(tt.role, service.AdminPermissionProxiesManage),
				nil,
			)

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			require.Equal(t, tt.wantStatus, rec.Code)
			require.Equal(t, tt.wantSecret, strings.Contains(rec.Body.String(), "secret-pass"))
			if tt.role == service.RoleAdmin {
				require.Zero(t, adminSvc.exportListCalls)
			}
		})
	}
}

func TestAdminAccountProxyOptionsProtectCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	adminSvc := &accountProxyOptionsAdminServiceStub{}
	registerAdminRoutesForPermissionTest(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{Account: adminhandler.NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)}},
		adminAuthForPermissionTest(service.RoleAdmin, service.AdminPermissionAccountsWrite),
		nil,
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/proxy-options", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, adminSvc.proxyOptionCalls)
	require.NotContains(t, rec.Body.String(), "password")
	require.NotContains(t, rec.Body.String(), "username")
}

func TestOllamaCloudUsageManagementRemainsSuperAdminOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	admin := router.Group("/api/v1/admin")
	admin.Use(gin.HandlerFunc(adminAuthForPermissionTest(service.RoleAdmin, service.AdminPermissionAccountsWrite)))
	registerAccountRoutes(
		admin,
		&handler.Handlers{Admin: &handler.AdminHandlers{Account: &adminhandler.AccountHandler{}}},
		servermiddleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() }),
	)

	for _, request := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/admin/accounts/ollama-cloud-usage/settings"},
		{http.MethodPut, "/api/v1/admin/accounts/ollama-cloud-usage/settings"},
		{http.MethodGet, "/api/v1/admin/accounts/7/ollama-cloud-usage"},
		{http.MethodPut, "/api/v1/admin/accounts/7/ollama-cloud-usage/session"},
		{http.MethodDelete, "/api/v1/admin/accounts/7/ollama-cloud-usage/session"},
		{http.MethodPut, "/api/v1/admin/accounts/7/ollama-cloud-usage/auto-refresh"},
		{http.MethodPost, "/api/v1/admin/accounts/7/ollama-cloud-usage/refresh"},
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(request.method, request.path, nil))
		require.Equal(t, http.StatusForbidden, rec.Code, "%s %s", request.method, request.path)
	}
}

type apiKeyRouteAdminServiceStub struct {
	service.AdminService
	updatedKeyID   int64
	updatedGroupID *int64
}

func (s *apiKeyRouteAdminServiceStub) AdminUpdateAPIKeyGroupID(_ context.Context, keyID int64, groupID *int64) (*service.AdminUpdateAPIKeyGroupIDResult, error) {
	s.updatedKeyID = keyID
	if groupID != nil {
		gid := *groupID
		s.updatedGroupID = &gid
	}
	key := &service.APIKey{ID: keyID, UserID: 7, Key: "sk-test", Name: "test", Status: service.StatusAPIKeyActive, GroupID: s.updatedGroupID}
	return &service.AdminUpdateAPIKeyGroupIDResult{APIKey: key}, nil
}

type proxyRouteAdminServiceStub struct {
	service.AdminService
	listCalls       int
	exportListCalls int
}

func newProxyRouteAdminServiceStub() *proxyRouteAdminServiceStub {
	return &proxyRouteAdminServiceStub{}
}

func (s *proxyRouteAdminServiceStub) ListProxiesWithAccountCount(context.Context, int, int, string, string, string, string, string) ([]service.ProxyWithAccountCount, int64, error) {
	s.listCalls++
	proxy := proxyRouteSecretProxy()
	return []service.ProxyWithAccountCount{{Proxy: proxy, AccountCount: 2}}, 1, nil
}

func (s *proxyRouteAdminServiceStub) ListProxies(context.Context, int, int, string, string, string, string, string) ([]service.Proxy, int64, error) {
	s.exportListCalls++
	return []service.Proxy{proxyRouteSecretProxy()}, 1, nil
}

func (s *proxyRouteAdminServiceStub) GetAllProxies(context.Context) ([]service.Proxy, error) {
	return []service.Proxy{proxyRouteSecretProxy()}, nil
}

func (s *proxyRouteAdminServiceStub) GetAllProxiesWithAccountCount(context.Context) ([]service.ProxyWithAccountCount, error) {
	proxy := proxyRouteSecretProxy()
	return []service.ProxyWithAccountCount{{Proxy: proxy, AccountCount: 2}}, nil
}

func (s *proxyRouteAdminServiceStub) GetProxy(context.Context, int64) (*service.Proxy, error) {
	proxy := proxyRouteSecretProxy()
	return &proxy, nil
}

func proxyRouteSecretProxy() service.Proxy {
	return service.Proxy{
		ID:       10,
		Name:     "shared",
		Protocol: "http",
		Host:     "proxy.example.test",
		Port:     8080,
		Username: "secret-user",
		Password: "secret-pass",
		Status:   service.StatusActive,
	}
}

type accountProxyOptionsAdminServiceStub struct {
	service.AdminService
	proxyOptionCalls int
}

func (s *accountProxyOptionsAdminServiceStub) GetAccountProxyOptions(context.Context) ([]service.Proxy, error) {
	s.proxyOptionCalls++
	return []service.Proxy{{
		ID:       10,
		Name:     "shared",
		Protocol: "http",
		Host:     "proxy.example.test",
		Port:     8080,
		Username: "secret-user",
		Password: "secret-pass",
		Status:   service.StatusActive,
	}}, nil
}
