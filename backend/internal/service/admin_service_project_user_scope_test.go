//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type scopedAdminUserRepoStub struct {
	userRepoStub

	scopedUsers []User
	listCalls   []UserListFilters
	listParams  []pagination.PaginationParams

	getByIDCalls               []int64
	getByIDIncludeDeletedCalls []int64
}

func (s *scopedAdminUserRepoStub) GetByID(ctx context.Context, id int64) (*User, error) {
	s.getByIDCalls = append(s.getByIDCalls, id)
	return s.userRepoStub.GetByID(ctx, id)
}

func (s *scopedAdminUserRepoStub) GetByIDIncludeDeleted(ctx context.Context, id int64) (*User, error) {
	s.getByIDIncludeDeletedCalls = append(s.getByIDIncludeDeletedCalls, id)
	return s.userRepoStub.GetByID(ctx, id)
}

func (s *scopedAdminUserRepoStub) ListWithFilters(_ context.Context, params pagination.PaginationParams, filters UserListFilters) ([]User, *pagination.PaginationResult, error) {
	s.listCalls = append(s.listCalls, filters)
	s.listParams = append(s.listParams, params)

	out := make([]User, 0, len(s.scopedUsers))
	for _, user := range s.scopedUsers {
		if filters.ID > 0 && user.ID != filters.ID {
			continue
		}
		out = append(out, user)
	}
	return out, &pagination.PaginationResult{
		Total:    int64(len(out)),
		Page:     params.Page,
		PageSize: params.PageSize,
	}, nil
}

func (s *scopedAdminUserRepoStub) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	return nil, nil
}

type scopedAdminGroupRepoStub struct {
	groupRepoStub
	groups map[int64]*Group
	calls  []int64
}

func (s *scopedAdminGroupRepoStub) GetByID(ctx context.Context, id int64) (*Group, error) {
	s.calls = append(s.calls, id)
	if group, ok := s.groups[id]; ok {
		clone := *group
		return &clone, nil
	}
	return nil, ErrGroupNotFound
}

func (s *scopedAdminGroupRepoStub) GetByIDLite(ctx context.Context, id int64) (*Group, error) {
	return s.GetByID(ctx, id)
}

func TestAdminService_GetUser_ProjectContextUsesScopedListFilter(t *testing.T) {
	repo := &scopedAdminUserRepoStub{
		scopedUsers: []User{{ID: 42, Email: "visible@example.com", Role: RoleUser}},
	}
	svc := &adminServiceImpl{userRepo: repo}

	user, err := svc.GetUser(WithProjectID(context.Background(), 7), 42)
	require.NoError(t, err)
	require.Equal(t, int64(42), user.ID)
	require.Empty(t, repo.getByIDCalls)
	require.Len(t, repo.listCalls, 1)
	require.Equal(t, int64(42), repo.listCalls[0].ID)
	require.False(t, repo.listCalls[0].IncludeDeleted)
	require.NotNil(t, repo.listCalls[0].IncludeSubscriptions)
	require.False(t, *repo.listCalls[0].IncludeSubscriptions)
	require.Equal(t, pagination.PaginationParams{Page: 1, PageSize: 1}, repo.listParams[0])
}

func TestAdminService_UpdateUser_ProjectContextRejectsInvisibleTarget(t *testing.T) {
	repo := &scopedAdminUserRepoStub{}
	svc := &adminServiceImpl{userRepo: repo}
	notes := "blocked"

	_, err := svc.UpdateUser(WithProjectID(context.Background(), 7), 42, &UpdateUserInput{Notes: &notes})
	require.ErrorIs(t, err, ErrUserNotFound)
	require.Empty(t, repo.getByIDCalls)
	require.Empty(t, repo.updated)
	require.Len(t, repo.listCalls, 1)
	require.Equal(t, int64(42), repo.listCalls[0].ID)
}

func TestAdminService_UpdateUser_ProjectAdminRejectsProjectAdminTarget(t *testing.T) {
	repo := &scopedAdminUserRepoStub{
		scopedUsers: []User{{ID: 42, Email: "admin@example.com", Role: RoleUser, ProjectRole: ProjectRoleAdmin}},
	}
	svc := &adminServiceImpl{userRepo: repo}
	notes := "blocked"
	ctx := WithAdminRole(WithProjectID(context.Background(), 7), RoleAdmin)

	_, err := svc.UpdateUser(ctx, 42, &UpdateUserInput{Notes: &notes})

	require.ErrorIs(t, err, ErrProjectAdminCannotManageAdminUser)
	require.Empty(t, repo.updated)
	require.Len(t, repo.listCalls, 1)
	require.Equal(t, int64(42), repo.listCalls[0].ID)
}

func TestAdminService_UpdateUser_ProjectAdminRejectsSuperAdminTarget(t *testing.T) {
	repo := &scopedAdminUserRepoStub{
		scopedUsers: []User{{ID: 42, Email: "root@example.com", Role: RoleSuperAdmin, ProjectRole: ProjectRoleAdmin}},
	}
	svc := &adminServiceImpl{userRepo: repo}
	notes := "blocked"
	ctx := WithAdminRole(WithProjectID(context.Background(), 7), RoleAdmin)

	_, err := svc.UpdateUser(ctx, 42, &UpdateUserInput{Notes: &notes})

	require.ErrorIs(t, err, ErrProjectAdminCannotManageAdminUser)
	require.Empty(t, repo.updated)
}

func TestAdminService_BatchUpdateConcurrency_ProjectAdminRejectsProjectAdminTarget(t *testing.T) {
	repo := &scopedAdminUserRepoStub{
		scopedUsers: []User{
			{ID: 41, Email: "user@example.com", Role: RoleUser, ProjectRole: ProjectRoleUser},
			{ID: 42, Email: "admin@example.com", Role: RoleUser, ProjectRole: ProjectRoleAdmin},
		},
	}
	svc := &adminServiceImpl{userRepo: repo}
	ctx := WithAdminRole(WithProjectID(context.Background(), 7), RoleAdmin)

	affected, err := svc.BatchUpdateConcurrency(ctx, []int64{41, 42}, 3, "set")

	require.ErrorIs(t, err, ErrProjectAdminCannotManageAdminUser)
	require.Zero(t, affected)
}

func TestAdminService_CreateUser_ProjectContextRejectsOutOfScopeAllowedGroups(t *testing.T) {
	userRepo := &scopedAdminUserRepoStub{userRepoStub: userRepoStub{nextID: 50}}
	groupRepo := &scopedAdminGroupRepoStub{groups: map[int64]*Group{
		10: {ID: 10, Name: "visible", Status: StatusActive},
	}}
	svc := &adminServiceImpl{userRepo: userRepo, groupRepo: groupRepo}

	_, err := svc.CreateUser(WithProjectID(context.Background(), 7), &CreateUserInput{
		Email:         "new@example.com",
		Password:      "password",
		AllowedGroups: []int64{10, 99},
	})

	require.ErrorIs(t, err, ErrGroupNotFound)
	require.Equal(t, []int64{10, 99}, groupRepo.calls)
	require.Empty(t, userRepo.created)
}

func TestAdminService_UpdateUser_ProjectContextRejectsOutOfScopeAllowedGroups(t *testing.T) {
	userRepo := &scopedAdminUserRepoStub{
		userRepoStub: userRepoStub{user: &User{ID: 42, Email: "visible@example.com", Role: RoleUser}},
		scopedUsers:  []User{{ID: 42, Email: "visible@example.com", Role: RoleUser}},
	}
	groupRepo := &scopedAdminGroupRepoStub{groups: map[int64]*Group{
		10: {ID: 10, Name: "visible", Status: StatusActive},
	}}
	svc := &adminServiceImpl{userRepo: userRepo, groupRepo: groupRepo}
	allowedGroups := []int64{99}

	_, err := svc.UpdateUser(WithProjectID(context.Background(), 7), 42, &UpdateUserInput{AllowedGroups: &allowedGroups})

	require.ErrorIs(t, err, ErrGroupNotFound)
	require.Equal(t, []int64{99}, groupRepo.calls)
	require.Empty(t, userRepo.updated)
}

func TestAdminService_UpdateUser_ProjectContextRejectsOutOfScopeGroupRates(t *testing.T) {
	userRepo := &scopedAdminUserRepoStub{
		userRepoStub: userRepoStub{user: &User{ID: 42, Email: "visible@example.com", Role: RoleUser}},
		scopedUsers:  []User{{ID: 42, Email: "visible@example.com", Role: RoleUser}},
	}
	groupRepo := &scopedAdminGroupRepoStub{groups: map[int64]*Group{
		10: {ID: 10, Name: "visible", Status: StatusActive},
	}}
	rate := 1.2
	svc := &adminServiceImpl{
		userRepo:          userRepo,
		groupRepo:         groupRepo,
		userGroupRateRepo: &userGroupRateRepoStubForGroupRate{},
	}

	_, err := svc.UpdateUser(WithProjectID(context.Background(), 7), 42, &UpdateUserInput{
		GroupRates: map[int64]*float64{10: &rate, 99: nil},
	})

	require.ErrorIs(t, err, ErrGroupNotFound)
	require.Contains(t, groupRepo.calls, int64(99))
	require.Empty(t, userRepo.updated)
}

func TestAdminService_CreateUser_ProjectContextRequiresGroupRepositoryForAllowedGroups(t *testing.T) {
	userRepo := &scopedAdminUserRepoStub{userRepoStub: userRepoStub{nextID: 50}}
	svc := &adminServiceImpl{userRepo: userRepo}

	_, err := svc.CreateUser(WithProjectID(context.Background(), 7), &CreateUserInput{
		Email:         "new@example.com",
		Password:      "password",
		AllowedGroups: []int64{10},
	})

	require.Error(t, err)
	require.Equal(t, "GROUP_REPOSITORY_UNAVAILABLE", infraerrors.Reason(err))
	require.Empty(t, userRepo.created)
}

func TestAdminService_GetUser_UnscopedUsesDirectLookup(t *testing.T) {
	repo := &scopedAdminUserRepoStub{
		userRepoStub: userRepoStub{user: &User{ID: 42, Email: "global@example.com", Role: RoleUser}},
	}
	svc := &adminServiceImpl{userRepo: repo}

	user, err := svc.GetUser(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, int64(42), user.ID)
	require.Equal(t, []int64{42}, repo.getByIDCalls)
	require.Empty(t, repo.listCalls)
}
