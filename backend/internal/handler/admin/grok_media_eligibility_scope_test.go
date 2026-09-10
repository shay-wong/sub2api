//go:build unit

package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGrokMediaEligibilityRequiresDirectAccountBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		for _, bound := range []bool{false, true} {
			name := method + "/group-only"
			accountIDs := []int64{}
			if bound {
				name = method + "/account-bound"
				accountIDs = []int64{12}
			}
			t.Run(name, func(t *testing.T) {
				account := &service.Account{
					ID: 12, Platform: service.PlatformGrok, Type: service.AccountTypeOAuth,
					GroupIDs: []int64{7}, Extra: map[string]any{"unrelated": "preserve"},
				}
				stub := &grokMediaEligibilityAdminServiceStub{stubAdminService: newStubAdminService(), account: account}
				permissions := service.NewPermissionService(
					&operatorPermissionRepoStub{adminScopes: map[int64]service.AdminResourceScope{
						101: {Mode: service.AdminResourceScopeRestricted, GroupIDs: []int64{7}, AccountIDs: accountIDs},
					}}, operatorUserRepoStub{}, operatorGroupRepoStub{},
				)
				h := NewAccountHandler(stub, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, permissions)
				router := gin.New()
				router.Use(func(c *gin.Context) {
					c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 101})
					c.Set(string(middleware.ContextKeyUserRole), service.RoleAdmin)
					c.Next()
				})
				router.GET("/accounts/:id/grok-media-eligibility", h.GetGrokMediaEligibility)
				router.PUT("/accounts/:id/grok-media-eligibility", h.UpdateGrokMediaEligibility)
				req := httptest.NewRequest(method, "/accounts/12/grok-media-eligibility", strings.NewReader(`{"mode":"enabled"}`))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				if bound {
					require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					if method == http.MethodPut {
						require.Equal(t, true, account.Extra[service.GrokMediaEligibleExtraKey])
					}
				} else {
					require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
					require.NotContains(t, account.Extra, service.GrokMediaEligibleExtraKey)
				}
				require.Equal(t, "preserve", account.Extra["unrelated"])
			})
		}
	}
}
