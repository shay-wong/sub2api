//go:build unit

package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIReferralRequiresDirectAccountBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"refresh", "invite"} {
		for _, bound := range []bool{false, true} {
			name := endpoint + "/group-only"
			var accountIDs []int64
			if bound {
				name = endpoint + "/account-bound"
				accountIDs = []int64{12}
			}
			t.Run(name, func(t *testing.T) {
				admin := newStubAdminService()
				account, err := admin.CreateAccount(context.Background(), &service.CreateAccountInput{
					Name: "referral-account", Platform: service.PlatformOpenAI,
					Type: service.AccountTypeOAuth, GroupIDs: []int64{7},
				})
				require.NoError(t, err)
				// Use the stored ID so the stub's account lookup exercises the real scope path.
				if bound {
					accountIDs = []int64{account.ID}
				}
				permissions := service.NewPermissionService(
					&operatorPermissionRepoStub{adminScopes: map[int64]service.AdminResourceScope{
						101: {Mode: service.AdminResourceScopeRestricted, GroupIDs: []int64{7}, AccountIDs: accountIDs},
					}}, operatorUserRepoStub{}, operatorGroupRepoStub{},
				)
				queries, writes := 0, 0
				referral := &referralHandlerStub{
					query: func(context.Context) (*service.OpenAIReferralEligibility, error) {
						queries++
						return &service.OpenAIReferralEligibility{ShouldShow: true}, nil
					},
					cache: func(context.Context, *service.OpenAIReferralEligibility) error {
						writes++
						return nil
					},
				}
				h := &OpenAIOAuthHandler{adminService: admin, permissionService: permissions, referralService: referral}
				router := gin.New()
				router.Use(func(c *gin.Context) {
					c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 101})
					c.Set(string(middleware.ContextKeyUserRole), service.RoleAdmin)
					c.Next()
				})
				router.POST("/:id/referrals/refresh", h.RefreshReferrals)
				router.POST("/:id/referrals/invite", h.SendReferralInvite)
				req := httptest.NewRequest(http.MethodPost, "/"+strconv.FormatInt(account.ID, 10)+"/referrals/"+endpoint,
					strings.NewReader(`{"email":"friend@example.com","program_id":"codex_referral_consumer","confirmed":true}`))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				if bound {
					require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					require.Equal(t, 1, queries)
					require.Equal(t, 1, writes)
					if endpoint == "invite" {
						require.Equal(t, 1, referral.sends)
					}
				} else {
					require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
					require.Zero(t, queries)
					require.Zero(t, writes)
					require.Zero(t, referral.sends)
				}
			})
		}
	}
}
