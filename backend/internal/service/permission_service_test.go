//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPermissionServiceSetOperatorPermissionsRejectsSuperAdmin(t *testing.T) {
	userRepo := &permissionUserRepoStub{
		user: &User{ID: 7, Email: "root@example.com", Role: RoleSuperAdmin, Status: StatusActive},
	}
	svc := NewPermissionService(&permissionOperatorRepoStub{}, userRepo, nil)

	updated, err := svc.SetOperatorPermissions(context.Background(), 7, RoleUser, nil, nil)

	require.ErrorIs(t, err, ErrPermissionCannotChangeAdmin)
	require.Nil(t, updated)
	require.False(t, userRepo.updated, "legacy permissions endpoint must not downgrade super admins")
}

func TestPermissionServiceSetOperatorPermissionsRejectsLegacyOperatorRole(t *testing.T) {
	userRepo := &permissionUserRepoStub{
		user: &User{ID: 8, Email: "user@example.com", Role: RoleUser, Status: StatusActive},
	}
	operatorRepo := &permissionOperatorRepoStub{}
	svc := NewPermissionService(operatorRepo, userRepo, nil)

	updated, err := svc.SetOperatorPermissions(context.Background(), 8, RoleOperator, []int64{1, 2}, nil)

	require.ErrorIs(t, err, ErrLegacyOperatorRoleDisabled)
	require.Nil(t, updated)
	require.False(t, userRepo.updated)
	require.False(t, operatorRepo.clearCalled)
}

func TestPermissionServiceSetOperatorPermissionsClearsLegacyScopeWhenDowngradingToUser(t *testing.T) {
	userRepo := &permissionUserRepoStub{
		user: &User{ID: 9, Email: "operator@example.com", Role: RoleOperator, Status: StatusActive},
	}
	operatorRepo := &permissionOperatorRepoStub{}
	svc := NewPermissionService(operatorRepo, userRepo, nil)

	updated, err := svc.SetOperatorPermissions(context.Background(), 9, RoleUser, []int64{10, 20}, nil)

	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, RoleUser, updated.Role)
	require.Empty(t, updated.GroupIDs)
	require.True(t, userRepo.updated)
	require.True(t, operatorRepo.clearCalled)
	require.Equal(t, int64(9), operatorRepo.clearUserID)
}

type permissionOperatorRepoStub struct {
	clearCalled bool
	clearUserID int64
}

func (permissionOperatorRepoStub) ListOperatorPermissionSubjects(context.Context) ([]OperatorPermissionSubject, error) {
	return nil, nil
}

func (permissionOperatorRepoStub) GetOperatorGroupIDs(context.Context, int64) ([]int64, error) {
	return nil, nil
}

func (permissionOperatorRepoStub) GetOperatorScopesByUserIDs(context.Context, []int64) (map[int64][]int64, error) {
	return map[int64][]int64{}, nil
}

func (permissionOperatorRepoStub) SetOperatorGroupIDs(context.Context, int64, []int64, *int64) error {
	return nil
}

func (r *permissionOperatorRepoStub) ClearOperatorGroupIDs(_ context.Context, userID int64) error {
	r.clearCalled = true
	r.clearUserID = userID
	return nil
}

type permissionUserRepoStub struct {
	UserRepository
	user    *User
	updated bool
}

func (r *permissionUserRepoStub) GetByID(context.Context, int64) (*User, error) {
	return r.user, nil
}

func (r *permissionUserRepoStub) Update(context.Context, *User) error {
	r.updated = true
	return nil
}
