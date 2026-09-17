package admin

import (
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionBulkAction_RejectsUnboundSubscriptionsBeforeMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name  string
		scope service.AdminResourceScope
	}{
		{name: "group binding grants no subscriptions", scope: service.AdminResourceScope{
			Mode: service.AdminResourceScopeRestricted, GroupIDs: []int64{10},
		}},
		{name: "mixed batch cannot mutate bound item first", scope: service.AdminResourceScope{
			Mode: service.AdminResourceScopeRestricted, SubscriptionIDs: []int64{1},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			permissionSvc := service.NewPermissionService(
				&operatorPermissionRepoStub{adminScopes: map[int64]service.AdminResourceScope{101: tc.scope}},
				operatorUserRepoStub{}, operatorGroupRepoStub{},
			)
			// No mutation service: every action must reject the whole batch before execution.
			handler := NewSubscriptionHandler(nil, permissionSvc)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 101})
				c.Set(string(middleware.ContextKeyUserRole), service.RoleAdmin)
				c.Next()
			})
			path := "/api/v1/admin/subscriptions/bulk-action"
			router.POST(path, handler.BulkAction)
			for _, body := range []string{
				`{"subscription_ids":[1,2],"action":"extend","days":7}`,
				`{"subscription_ids":[1,2],"action":"reset_quota","daily":true}`,
				`{"subscription_ids":[1,2],"action":"revoke"}`,
				`{"subscription_ids":[1,2],"action":"restore"}`,
			} {
				rec := bulkActionHandlerRequest(router, path, body, "scope-denied")
				require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSubscriptionBulkAction_DirectBindingAndRevokedReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := service.DefaultIdempotencyConfig()
	cfg.ObserveOnly = false
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(newMemoryIdempotencyRepoStub(), cfg))
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(nil) })
	expiresAt := time.Now().AddDate(0, 0, 30)
	repo := &bulkActionHandlerSubscriptionRepo{sub: &service.UserSubscription{ID: 1, UserID: 1, GroupID: 10, ExpiresAt: expiresAt}}
	subscriptionSvc := service.NewSubscriptionService(nil, repo, nil, nil, nil)
	t.Cleanup(subscriptionSvc.Stop)
	permissionRepo := &operatorPermissionRepoStub{adminScopes: map[int64]service.AdminResourceScope{101: {
		Mode: service.AdminResourceScopeRestricted, SubscriptionIDs: []int64{1},
	}}}
	permissionSvc := service.NewPermissionService(permissionRepo, operatorUserRepoStub{}, operatorGroupRepoStub{})
	handler := NewSubscriptionHandler(subscriptionSvc, permissionSvc)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 101})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleAdmin)
		c.Next()
	})
	path := "/api/v1/admin/subscriptions/bulk-action"
	router.POST(path, handler.BulkAction)
	body := `{"subscription_ids":[1],"action":"extend","days":7}`

	first := bulkActionHandlerRequest(router, path, body, "bound-once")
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	replayed := bulkActionHandlerRequest(router, path, body, "bound-once")
	require.Equal(t, http.StatusOK, replayed.Code, replayed.Body.String())
	require.Equal(t, "true", replayed.Header().Get("X-Idempotency-Replayed"))
	require.Equal(t, 1, repo.extendCalls)
	require.Equal(t, expiresAt.AddDate(0, 0, 7), repo.sub.ExpiresAt)

	permissionRepo.adminScopes[101] = service.AdminResourceScope{
		Mode: service.AdminResourceScopeRestricted, GroupIDs: []int64{10},
	}
	revoked := bulkActionHandlerRequest(router, path, body, "bound-once")
	require.Equal(t, http.StatusForbidden, revoked.Code, revoked.Body.String())
	require.Empty(t, revoked.Header().Get("X-Idempotency-Replayed"))
	require.Equal(t, 1, repo.extendCalls)
}
