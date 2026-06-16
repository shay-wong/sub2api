package admin

import (
	"bytes"
	"context"
	"encoding/json"
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
	router.POST("/api/v1/admin/accounts/bulk-update", handler.BulkUpdate)
	return router
}

func TestOperatorAccountListUsesAssignedGroupScope(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.accounts = []service.Account{
		{ID: 1, Name: "visible-a", Status: service.StatusActive, GroupIDs: []int64{10}},
		{ID: 2, Name: "hidden", Status: service.StatusActive, GroupIDs: []int64{30}},
		{ID: 3, Name: "visible-b", Status: service.StatusActive, GroupIDs: []int64{20}},
	}
	router := newOperatorAccountScopeRouter(adminSvc, []int64{10, 20})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts?page=1&page_size=20", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data struct {
			Total int `json:"total"`
			Items []struct {
				ID int64 `json:"id"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 2, resp.Data.Total)
	require.Equal(t, []int64{1, 3}, []int64{resp.Data.Items[0].ID, resp.Data.Items[1].ID})
}

func TestOperatorAccountListTrimsOutOfScopeGroups(t *testing.T) {
	adminSvc := newStubAdminService()
	now := time.Now().UTC()
	adminSvc.accounts = []service.Account{
		{
			ID:        1,
			Name:      "shared",
			Status:    service.StatusActive,
			GroupIDs:  []int64{10, 30},
			CreatedAt: now,
			UpdatedAt: now,
			Groups: []*service.Group{
				{ID: 10, Name: "visible", Status: service.StatusActive},
				{ID: 30, Name: "hidden", Status: service.StatusActive},
			},
			AccountGroups: []service.AccountGroup{
				{AccountID: 1, GroupID: 10, Group: &service.Group{ID: 10, Name: "visible", Status: service.StatusActive}},
				{AccountID: 1, GroupID: 30, Group: &service.Group{ID: 30, Name: "hidden", Status: service.StatusActive}},
			},
		},
	}
	router := newOperatorAccountScopeRouter(adminSvc, []int64{10})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts?page=1&page_size=20", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data struct {
			Items []struct {
				GroupIDs []int64 `json:"group_ids"`
				Groups   []struct {
					ID   int64  `json:"id"`
					Name string `json:"name"`
				} `json:"groups"`
				AccountGroups []struct {
					GroupID int64 `json:"group_id"`
					Group   *struct {
						ID   int64  `json:"id"`
						Name string `json:"name"`
					} `json:"group"`
				} `json:"account_groups"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Data.Items, 1)
	item := resp.Data.Items[0]
	require.Equal(t, []int64{10}, item.GroupIDs)
	require.Len(t, item.Groups, 1)
	require.Equal(t, int64(10), item.Groups[0].ID)
	require.Equal(t, "visible", item.Groups[0].Name)
	require.Len(t, item.AccountGroups, 1)
	require.Equal(t, int64(10), item.AccountGroups[0].GroupID)
	require.NotNil(t, item.AccountGroups[0].Group)
	require.Equal(t, "visible", item.AccountGroups[0].Group.Name)
	require.NotContains(t, rec.Body.String(), "hidden")
}

func TestOperatorAccountDetailTrimsOutOfScopeGroups(t *testing.T) {
	adminSvc := newStubAdminService()
	now := time.Now().UTC()
	adminSvc.accounts = []service.Account{
		{
			ID:        1,
			Name:      "shared",
			Status:    service.StatusActive,
			GroupIDs:  []int64{10, 30},
			CreatedAt: now,
			UpdatedAt: now,
			Groups: []*service.Group{
				{ID: 10, Name: "visible", Status: service.StatusActive},
				{ID: 30, Name: "hidden", Status: service.StatusActive},
			},
			AccountGroups: []service.AccountGroup{
				{AccountID: 1, GroupID: 10, Group: &service.Group{ID: 10, Name: "visible", Status: service.StatusActive}},
				{AccountID: 1, GroupID: 30, Group: &service.Group{ID: 30, Name: "hidden", Status: service.StatusActive}},
			},
		},
	}
	router := newOperatorAccountScopeRouter(adminSvc, []int64{10})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/1", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data struct {
			GroupIDs []int64 `json:"group_ids"`
			Groups   []struct {
				ID   int64  `json:"id"`
				Name string `json:"name"`
			} `json:"groups"`
			AccountGroups []struct {
				GroupID int64 `json:"group_id"`
				Group   *struct {
					ID   int64  `json:"id"`
					Name string `json:"name"`
				} `json:"group"`
			} `json:"account_groups"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, []int64{10}, resp.Data.GroupIDs)
	require.Len(t, resp.Data.Groups, 1)
	require.Equal(t, "visible", resp.Data.Groups[0].Name)
	require.Len(t, resp.Data.AccountGroups, 1)
	require.Equal(t, int64(10), resp.Data.AccountGroups[0].GroupID)
	require.NotContains(t, rec.Body.String(), "hidden")
}

func TestOperatorAccountDetailRejectsOutOfScopeAccount(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.accounts = []service.Account{{ID: 99, Name: "hidden", Status: service.StatusActive, GroupIDs: []int64{30}}}
	router := newOperatorAccountScopeRouter(adminSvc, []int64{10})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/99", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "OPERATOR_ACCOUNT_FORBIDDEN")
}

func TestOperatorBulkUpdateRejectsFilterOutsideScope(t *testing.T) {
	adminSvc := newStubAdminService()
	router := newOperatorAccountScopeRouter(adminSvc, []int64{10})
	body, _ := json.Marshal(map[string]any{
		"filters":     map[string]any{"group": "30"},
		"schedulable": true,
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/bulk-update", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "OPERATOR_SCOPE_FORBIDDEN")
}
