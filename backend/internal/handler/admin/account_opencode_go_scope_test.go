package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type scopedOpenCodeGoUsageRepo struct {
	openCodeGoUsageHandlerTestRepo
	resolveCalls int
}

func (r *scopedOpenCodeGoUsageRepo) ListOpenCodeGoUsageGroupAccounts(ctx context.Context, anchors []*service.Account) ([]service.Account, error) {
	r.resolveCalls++
	return r.openCodeGoUsageHandlerTestRepo.ListOpenCodeGoUsageGroupAccounts(ctx, anchors)
}

func TestOpenCodeGoUsageRestrictedAccountResponsesDoNotResolveSiblings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, target := range []string{"/accounts", "/accounts/7"} {
		t.Run(target, func(t *testing.T) {
			visible := &service.Account{
				ID: 7, Name: "directly bound", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
				Credentials: map[string]any{"base_url": "https://opencode.ai/zen/go/v1", "api_key": "shared-key"},
				Extra:       map[string]any{}, Status: service.StatusActive,
			}
			unbound := *visible
			unbound.ID = 8
			unbound.Extra = map[string]any{service.OpenCodeGoUsageAutoRefreshExtraKey: true}
			repo := &scopedOpenCodeGoUsageRepo{openCodeGoUsageHandlerTestRepo: openCodeGoUsageHandlerTestRepo{accounts: []*service.Account{visible, &unbound}}}
			usage := service.NewOpenCodeGoUsageService(repo, nil, nil)
			t.Cleanup(usage.Stop)
			adminService := newStubAdminService()
			adminService.accounts = []service.Account{*visible}
			adminService.getAccountResult = visible
			permissions := service.NewPermissionService(&operatorPermissionRepoStub{adminScopes: map[int64]service.AdminResourceScope{
				101: {Mode: service.AdminResourceScopeRestricted, AccountIDs: []int64{7}},
			}}, operatorUserRepoStub{}, operatorGroupRepoStub{})
			h := NewAccountHandler(adminService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, permissions)
			h.SetOpenCodeGoUsageService(usage)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 101})
				c.Set(string(middleware.ContextKeyUserRole), service.RoleAdmin)
				c.Request = c.Request.WithContext(service.WithAdminRole(c.Request.Context(), service.RoleAdmin))
				c.Next()
			})
			router.GET("/accounts", h.List)
			router.GET("/accounts/:id", h.GetByID)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Zero(t, repo.resolveCalls, "direct account access must not resolve unbound key siblings")
			require.NotContains(t, visible.Extra, service.OpenCodeGoUsageAutoRefreshExtraKey)
		})
	}
}
