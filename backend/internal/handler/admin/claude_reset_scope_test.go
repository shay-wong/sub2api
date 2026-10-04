//go:build unit

package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type claudeResetScopeStub struct {
	claudeResetHandlerStub
	queries int
}

func (s *claudeResetScopeStub) Query(context.Context, int64) (*service.ClaudeResetCredits, error) {
	s.queries++
	return &service.ClaudeResetCredits{Credits: []service.ClaudeResetCredit{}}, nil
}

func TestClaudeResetRequiresDirectAccountBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		for _, access := range []string{"group-only", "account-bound", "all", "super-admin"} {
			t.Run(method+"/"+access, func(t *testing.T) {
				admin := newStubAdminService()
				account, err := admin.CreateAccount(context.Background(), &service.CreateAccountInput{
					Name: "claude-account", Platform: service.PlatformAnthropic,
					Type: service.AccountTypeOAuth, GroupIDs: []int64{7},
				})
				require.NoError(t, err)
				scope := service.AdminResourceScope{Mode: service.AdminResourceScopeRestricted, GroupIDs: []int64{7}}
				role := service.RoleAdmin
				switch access {
				case "account-bound":
					scope.AccountIDs = []int64{account.ID}
				case "all":
					scope.Mode = service.AdminResourceScopeAll
				case "super-admin":
					role = service.RoleSuperAdmin
				}
				permissions := service.NewPermissionService(
					&operatorPermissionRepoStub{adminScopes: map[int64]service.AdminResourceScope{101: scope}},
					operatorUserRepoStub{}, operatorGroupRepoStub{},
				)
				stub := &claudeResetScopeStub{}
				h := &AccountHandler{adminService: admin, permissionService: permissions, claudeResetCredits: stub}
				router := gin.New()
				router.Use(func(c *gin.Context) {
					c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 101})
					c.Set(string(middleware.ContextKeyUserRole), role)
					c.Next()
				})
				router.GET("/accounts/:id/claude/reset-credits", h.ClaudeResetCredits)
				router.POST("/accounts/:id/claude/reset-credits/redeem", h.RedeemClaudeResetCredit)
				path := "/accounts/" + strconv.FormatInt(account.ID, 10) + "/claude/reset-credits"
				if method == http.MethodPost {
					path += "/redeem"
				}
				req := httptest.NewRequest(method, path, nil)
				req.Header.Set("Idempotency-Key", "confirmed-operation")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				if access == "group-only" {
					require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
					require.Zero(t, stub.queries)
					require.Zero(t, stub.calls)
				} else {
					require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					if method == http.MethodGet {
						require.Equal(t, 1, stub.queries)
						require.Zero(t, stub.calls)
					} else {
						require.Equal(t, 1, stub.calls)
						require.Equal(t, "confirmed-operation", stub.key)
					}
				}
			})
		}
	}
}
