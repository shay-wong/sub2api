package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type subscriptionScopeRepo struct {
	service.UserSubscriptionRepository
	subscriptions []service.UserSubscription
}

func (r subscriptionScopeRepo) ListByGroupIDAndIDs(_ context.Context, groupID int64, params pagination.PaginationParams, ids []int64) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	allowed := int64Set(ids)
	items := make([]service.UserSubscription, 0, len(r.subscriptions))
	for _, subscription := range r.subscriptions {
		if subscription.GroupID != groupID {
			continue
		}
		if _, ok := allowed[subscription.ID]; ok {
			items = append(items, subscription)
		}
	}
	return items, &pagination.PaginationResult{Total: int64(len(items)), Page: params.Page, PageSize: params.PageSize}, nil
}

func TestRestrictedAdminGroupSubscriptionsUseDirectSubscriptionBindings(t *testing.T) {
	repo := subscriptionScopeRepo{subscriptions: []service.UserSubscription{
		{ID: 1, GroupID: 10},
		{ID: 2, GroupID: 10},
		{ID: 3, GroupID: 20},
	}}
	subscriptionSvc := service.NewSubscriptionService(nil, repo, nil, nil, nil)
	permissionSvc := service.NewPermissionService(
		&operatorPermissionRepoStub{adminScopes: map[int64]service.AdminResourceScope{101: {
			Mode:            service.AdminResourceScopeRestricted,
			GroupIDs:        []int64{10},
			SubscriptionIDs: []int64{2, 3},
		}}},
		operatorUserRepoStub{},
		operatorGroupRepoStub{},
	)
	handler := NewSubscriptionHandler(subscriptionSvc, permissionSvc)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 101})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleAdmin)
		c.Next()
	})
	router.GET("/api/v1/admin/groups/:id/subscriptions", handler.ListByGroup)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/groups/10/subscriptions?page=1&page_size=20", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), `"id":1`)
	require.Contains(t, rec.Body.String(), `"id":2`)
	require.NotContains(t, rec.Body.String(), `"id":3`)
}
