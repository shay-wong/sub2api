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

type permissionOperatorRepoStub struct{}

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

func (permissionOperatorRepoStub) ClearOperatorGroupIDs(context.Context, int64) error {
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
