//go:build unit

package service

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

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
	svc := &adminServiceImpl{userRepo: repo, authCacheInvalidator: invalidator}

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
}

func TestAdminServiceUpdateAdminAccessDemotionClearsPermissions(t *testing.T) {
	base := &userRepoStub{user: &User{
		ID:               42,
		Email:            "admin@example.com",
		Role:             RoleAdmin,
		AdminPermissions: []string{AdminPermissionUsageRead},
	}}
	repo := &rpmUserRepoStub{userRepoStub: base}
	svc := &adminServiceImpl{userRepo: repo}

	updated, err := svc.UpdateUserAdminAccess(context.Background(), 42, &UpdateUserAdminAccessInput{
		Role:             RoleUser,
		AdminPermissions: []string{AdminPermissionAccountsWrite},
		ActorAdminID:     7,
	})

	require.NoError(t, err)
	require.Equal(t, RoleUser, updated.Role)
	require.Empty(t, updated.AdminPermissions)
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
