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
	scopes map[int64][]int64
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

type operatorUserRepoStub struct{}

func (operatorUserRepoStub) Create(context.Context, *service.User) error { return nil }
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
func (operatorUserRepoStub) Update(context.Context, *service.User) error          { return nil }
func (operatorUserRepoStub) Delete(context.Context, int64) error                  { return nil }
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
func (operatorUserRepoStub) DeductBalance(context.Context, int64, float64) error { return nil }
func (operatorUserRepoStub) UpdateConcurrency(context.Context, int64, int) error { return nil }
func (operatorUserRepoStub) BatchSetConcurrency(context.Context, []int64, int) (int, error) {
	return 0, nil
}
func (operatorUserRepoStub) BatchAddConcurrency(context.Context, []int64, int) (int, error) {
	return 0, nil
}
func (operatorUserRepoStub) ExistsByEmail(context.Context, string) (bool, error) { return false, nil }
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
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, permissionSvc)
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
