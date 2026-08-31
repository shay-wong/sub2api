//go:build unit

package service

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

type adminResourceScopeRepoStub struct {
	role        string
	permissions []string
	scope       AdminResourceScope
}

func (s *adminResourceScopeRepoStub) GetAdminResourceScope(context.Context, int64) (AdminResourceScope, error) {
	return UnrestrictedAdminResourceScope(), nil
}

func (s *adminResourceScopeRepoStub) GetAdminResourceScopesByUserIDs(context.Context, []int64) (map[int64]AdminResourceScope, error) {
	return map[int64]AdminResourceScope{}, nil
}

func (s *adminResourceScopeRepoStub) UpdateUserAdminAccess(_ context.Context, _ int64, role string, permissions []string, scope AdminResourceScope, _ *int64) error {
	s.role = role
	s.permissions = append([]string(nil), permissions...)
	s.scope = scope
	return nil
}

func (s *adminResourceScopeRepoStub) BindAdminResource(context.Context, int64, string, int64, *int64) error {
	return nil
}

func TestAdminServiceRegularInputsDoNotExposeAdminAccess(t *testing.T) {
	for _, inputType := range []reflect.Type{
		reflect.TypeOf(CreateUserInput{}),
		reflect.TypeOf(UpdateUserInput{}),
	} {
		_, hasRole := inputType.FieldByName("Role")
		require.False(t, hasRole, "%s must not change roles", inputType.Name())
		_, hasPermissions := inputType.FieldByName("AdminPermissions")
		require.False(t, hasPermissions, "%s must not change admin permissions", inputType.Name())
	}
}

func TestAdminServiceCreateUserDefaultsToUserRole(t *testing.T) {
	repo := &userRepoStub{nextID: 31}
	svc := &adminServiceImpl{userRepo: repo}

	user, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "plain@test.com",
		Password: "strong-pass",
	})

	require.NoError(t, err)
	require.Equal(t, RoleUser, user.Role)
	require.Empty(t, user.AdminPermissions)
}

func TestAdminServiceUpdateUserPreservesAdminAccess(t *testing.T) {
	base := &userRepoStub{user: &User{
		ID:               42,
		Email:            "admin@example.com",
		Role:             RoleAdmin,
		AdminPermissions: []string{AdminPermissionUsageRead},
	}}
	repo := &rpmUserRepoStub{userRepoStub: base}
	svc := &adminServiceImpl{userRepo: repo, redeemCodeRepo: &redeemRepoStub{}}

	newName := "renamed"
	updated, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{Username: &newName})

	require.NoError(t, err)
	require.Equal(t, RoleAdmin, updated.Role)
	require.Equal(t, []string{AdminPermissionUsageRead}, updated.AdminPermissions)
	require.NotNil(t, repo.lastUpdated)
}

func TestAdminServiceUpdateAdminAccessNormalizesAndInvalidates(t *testing.T) {
	base := &userRepoStub{user: &User{ID: 42, Email: "user@example.com", Role: RoleUser}}
	repo := &rpmUserRepoStub{userRepoStub: base}
	invalidator := &authCacheInvalidatorStub{}
	scopeRepo := &adminResourceScopeRepoStub{}
	svc := &adminServiceImpl{
		userRepo:             repo,
		authCacheInvalidator: invalidator,
		permissionService:    &PermissionService{resourceScopeRepo: scopeRepo},
	}

	updated, err := svc.UpdateUserAdminAccess(context.Background(), 42, &UpdateUserAdminAccessInput{
		Role: RoleAdmin,
		AdminPermissions: []string{
			AdminPermissionUsageRead,
			AdminPermissionDashboardRead,
			AdminPermissionUsageRead,
		},
		ActorAdminID: 7,
	})

	require.NoError(t, err)
	require.Equal(t, RoleAdmin, updated.Role)
	require.Equal(t, []string{AdminPermissionDashboardRead, AdminPermissionUsageRead}, updated.AdminPermissions)
	require.Equal(t, []int64{42}, invalidator.userIDs)
	require.Equal(t, RoleAdmin, scopeRepo.role)
	require.Equal(t, updated.AdminPermissions, scopeRepo.permissions)
	require.Equal(t, UnrestrictedAdminResourceScope(), scopeRepo.scope)
}

func TestAdminServiceUpdateAdminAccessDemotionClearsPermissions(t *testing.T) {
	base := &userRepoStub{user: &User{
		ID:               42,
		Email:            "admin@example.com",
		Role:             RoleAdmin,
		AdminPermissions: []string{AdminPermissionUsageRead},
	}}
	repo := &rpmUserRepoStub{userRepoStub: base}
	scopeRepo := &adminResourceScopeRepoStub{}
	svc := &adminServiceImpl{
		userRepo:          repo,
		permissionService: &PermissionService{resourceScopeRepo: scopeRepo},
	}

	updated, err := svc.UpdateUserAdminAccess(context.Background(), 42, &UpdateUserAdminAccessInput{
		Role:             RoleUser,
		AdminPermissions: []string{AdminPermissionAccountsWrite},
		ActorAdminID:     7,
	})

	require.NoError(t, err)
	require.Equal(t, RoleUser, updated.Role)
	require.Empty(t, updated.AdminPermissions)
	require.Equal(t, RoleUser, scopeRepo.role)
	require.Empty(t, scopeRepo.permissions)
	require.Equal(t, UnrestrictedAdminResourceScope(), scopeRepo.scope)
}

func TestAdminServiceUpdateAdminAccessRejectsSuperAdmin(t *testing.T) {
	base := &userRepoStub{user: &User{ID: 42, Email: "root@example.com", Role: RoleSuperAdmin}}
	repo := &rpmUserRepoStub{userRepoStub: base}
	svc := &adminServiceImpl{userRepo: repo}

	_, err := svc.UpdateUserAdminAccess(context.Background(), 42, &UpdateUserAdminAccessInput{
		Role:         RoleAdmin,
		ActorAdminID: 7,
	})

	require.Error(t, err)
	require.Nil(t, repo.lastUpdated)
}

func TestAdminServiceUpdateAdminAccessRejectsUnknownPermission(t *testing.T) {
	base := &userRepoStub{user: &User{ID: 42, Email: "user@example.com", Role: RoleUser}}
	repo := &rpmUserRepoStub{userRepoStub: base}
	svc := &adminServiceImpl{userRepo: repo}

	_, err := svc.UpdateUserAdminAccess(context.Background(), 42, &UpdateUserAdminAccessInput{
		Role:             RoleAdmin,
		AdminPermissions: []string{"admin.unknown"},
		ActorAdminID:     7,
	})

	require.Error(t, err)
	require.Nil(t, repo.lastUpdated)
}

func TestAdminServiceUpdateAndDeleteProtectSuperAdmin(t *testing.T) {
	base := &userRepoStub{user: &User{ID: 42, Email: "root@example.com", Role: RoleSuperAdmin}}
	repo := &rpmUserRepoStub{userRepoStub: base}
	svc := &adminServiceImpl{userRepo: repo, redeemCodeRepo: &redeemRepoStub{}}

	_, updateErr := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{Status: StatusDisabled})
	require.Error(t, updateErr)

	deleteErr := svc.DeleteUser(context.Background(), 42)
	require.Error(t, deleteErr)
	require.Empty(t, base.deletedIDs)
}
