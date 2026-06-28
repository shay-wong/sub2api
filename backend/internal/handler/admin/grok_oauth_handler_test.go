//go:build unit

package admin

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type grokQuotaHandlerAccountRepo struct {
	service.AccountRepository
	account *service.Account
	updates map[int64]map[string]any
}

func (r *grokQuotaHandlerAccountRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	if r.account != nil && r.account.ID == id {
		return r.account, nil
	}
	return nil, service.ErrAccountNotFound
}

func (r *grokQuotaHandlerAccountRepo) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	if r.updates == nil {
		r.updates = make(map[int64]map[string]any)
	}
	r.updates[id] = updates
	return nil
}

type grokQuotaHandlerUpstream struct {
	resp     *http.Response
	lastReq  *http.Request
	lastBody []byte
}

func (u *grokQuotaHandlerUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.lastReq = req
	if req.Body != nil {
		u.lastBody, _ = io.ReadAll(req.Body)
	}
	return u.resp, nil
}

func (u *grokQuotaHandlerUpstream) DoWithTLS(
	req *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestGrokOAuthHandlerQueryQuotaProbesUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &grokQuotaHandlerAccountRepo{account: &service.Account{
		ID:          42,
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "access-token",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}}
	upstream := &grokQuotaHandlerUpstream{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"X-Ratelimit-Limit-Requests":     []string{"10"},
			"X-Ratelimit-Remaining-Requests": []string{"8"},
		},
		Body: io.NopCloser(strings.NewReader(`{"id":"resp_probe"}`)),
	}}
	quotaService := service.NewGrokQuotaService(repo, nil, service.NewGrokTokenProvider(repo, nil), upstream)
	adminSvc := newStubAdminService()
	adminSvc.accounts = []service.Account{*repo.account}
	handler := NewGrokOAuthHandler(nil, adminSvc, nil, quotaService)

	router := gin.New()
	router.GET("/api/v1/admin/grok/accounts/:id/quota", handler.QueryQuota)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/grok/accounts/42/quota", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"source":"active_probe"`)
	require.Contains(t, rec.Body.String(), `"headers_observed":true`)
	require.NotContains(t, rec.Body.String(), "access-token")
	require.Equal(t, xai.DefaultBaseURL+"/responses", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer access-token", upstream.lastReq.Header.Get("Authorization"))
	require.Contains(t, string(upstream.lastBody), `"store":false`)
	require.NotNil(t, repo.updates[42])
}

func TestGrokOAuthHandlerResetQuotaReturnsUnsupported(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &grokQuotaHandlerAccountRepo{account: &service.Account{
		ID:       43,
		Platform: service.PlatformGrok,
		Type:     service.AccountTypeOAuth,
	}}
	quotaService := service.NewGrokQuotaService(repo, nil, nil, nil)
	adminSvc := newStubAdminService()
	adminSvc.accounts = []service.Account{*repo.account}
	handler := NewGrokOAuthHandler(nil, adminSvc, nil, quotaService)

	router := gin.New()
	router.POST("/api/v1/admin/grok/accounts/:id/reset-quota", handler.ResetQuota)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/grok/accounts/43/reset-quota", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotImplemented, rec.Code)
	require.Contains(t, rec.Body.String(), `"reason":"GROK_QUOTA_RESET_UNSUPPORTED"`)
	require.NotContains(t, rec.Body.String(), "access-token")
}

func TestGrokOAuthHandlerRuntimeSanityDoesNotExposeSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(xai.EnvBaseURL, "http://127.0.0.1:8080/v1?access_token=secret")
	t.Setenv(xai.EnvClientID, "client-secret-like-value")

	handler := NewGrokOAuthHandler(nil, nil, nil, nil)
	router := gin.New()
	router.GET("/api/v1/admin/grok/runtime-sanity", handler.RuntimeSanity)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/grok/runtime-sanity", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"public_gateway_scope":"responses_only"`)
	require.Contains(t, rec.Body.String(), `"valid":false`)
	require.NotContains(t, rec.Body.String(), "access_token")
	require.NotContains(t, rec.Body.String(), "secret")
	require.NotContains(t, rec.Body.String(), "client-secret-like-value")
}

func newProjectGrokOAuthScopeRouter(adminSvc *stubAdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewGrokOAuthHandler(service.NewGrokOAuthService(nil, nil), adminSvc, nil, nil)
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 101})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleAdmin)
		c.Request = c.Request.WithContext(service.WithProjectID(c.Request.Context(), 169))
		c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), service.DefaultProjectAdminPermissions()))
		c.Next()
	})
	router.POST("/api/v1/admin/grok/oauth/auth-url", handler.GenerateAuthURL)
	router.POST("/api/v1/admin/grok/oauth/create-from-oauth", handler.CreateAccountFromOAuth)
	router.GET("/api/v1/admin/grok/accounts/:id/quota", handler.QueryQuota)
	return router
}

func TestProjectGrokOAuthRoutesAllowProjectScopedProxyUse(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.proxies = []service.Proxy{{
		ID:        4,
		ProjectID: 1,
		Name:      "shared-visible-proxy",
		Protocol:  "http",
		Host:      "127.0.0.1",
		Port:      8080,
		Status:    service.StatusActive,
	}}
	router := newProjectGrokOAuthScopeRouter(adminSvc)

	cases := []struct {
		name       string
		path       string
		body       string
		wantStatus int
		wantReason string
	}{
		{
			name:       "auth_url_without_account",
			path:       "/api/v1/admin/grok/oauth/auth-url",
			body:       `{"proxy_id":4}`,
			wantStatus: http.StatusBadRequest,
			wantReason: "proxy repository is not available",
		},
		{
			name:       "create_from_oauth",
			path:       "/api/v1/admin/grok/oauth/create-from-oauth",
			body:       `{"session_id":"session","code":"code","group_ids":[2],"proxy_id":4}`,
			wantStatus: http.StatusBadRequest,
			wantReason: "GROK_OAUTH_SESSION_NOT_FOUND",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)

			require.Equal(t, tc.wantStatus, rec.Code)
			require.Contains(t, rec.Body.String(), tc.wantReason)
			require.NotContains(t, rec.Body.String(), "OPERATOR_PROXY_FORBIDDEN")
		})
	}
	require.Empty(t, adminSvc.createdAccounts)
}

func TestProjectGrokQuotaChecksAccountScopeBeforeQuotaService(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.accounts = nil
	adminSvc.hideMissingAccounts = true
	router := newProjectGrokOAuthScopeRouter(adminSvc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/grok/accounts/42/quota", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), "ACCOUNT_NOT_FOUND")
}
