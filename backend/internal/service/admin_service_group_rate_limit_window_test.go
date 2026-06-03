//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type userGroupRateLimitWindowRepoStubForAdmin struct {
	listByUserID int64
	listRecords  []UserGroupRateLimitWindowRecord
	listErr      error

	resetUserID  int64
	resetGroupID int64
	resetRecord  *UserGroupRateLimitWindowRecord
	resetErr     error
}

func (s *userGroupRateLimitWindowRepoStubForAdmin) Get(context.Context, int64, int64) (*UserGroupRateLimitWindowRecord, error) {
	panic("unexpected Get call")
}

func (s *userGroupRateLimitWindowRepoStubForAdmin) ListByUser(_ context.Context, userID int64) ([]UserGroupRateLimitWindowRecord, error) {
	s.listByUserID = userID
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.listRecords, nil
}

func (s *userGroupRateLimitWindowRepoStubForAdmin) ListByGroup(context.Context, int64, pagination.PaginationParams) ([]UserGroupRateLimitWindowRecord, *pagination.PaginationResult, error) {
	panic("unexpected ListByGroup call")
}

func (s *userGroupRateLimitWindowRepoStubForAdmin) IncrementWithWindowReset(context.Context, int64, int64, float64, time.Time) error {
	panic("unexpected IncrementWithWindowReset call")
}

func (s *userGroupRateLimitWindowRepoStubForAdmin) Reset(_ context.Context, userID, groupID int64) (*UserGroupRateLimitWindowRecord, error) {
	s.resetUserID = userID
	s.resetGroupID = groupID
	if s.resetErr != nil {
		return nil, s.resetErr
	}
	if s.resetRecord != nil {
		return s.resetRecord, nil
	}
	return &UserGroupRateLimitWindowRecord{UserID: userID, GroupID: groupID}, nil
}

func TestAdminService_ListUserGroupRateLimitWindows(t *testing.T) {
	t.Run("validates user then lists windows", func(t *testing.T) {
		repo := &userGroupRateLimitWindowRepoStubForAdmin{
			listRecords: []UserGroupRateLimitWindowRecord{
				{UserID: 7, GroupID: 2, GroupName: "pro", RateLimit5h: 10, Usage5hUSD: 3},
			},
		}
		svc := &adminServiceImpl{
			userRepo:               &userRepoStub{user: &User{ID: 7}},
			userGroupRateLimitRepo: repo,
		}

		got, err := svc.ListUserGroupRateLimitWindows(context.Background(), 7)
		require.NoError(t, err)
		require.Equal(t, int64(7), repo.listByUserID)
		require.Len(t, got, 1)
		require.Equal(t, int64(2), got[0].GroupID)
	})

	t.Run("rejects invalid user id before repo call", func(t *testing.T) {
		repo := &userGroupRateLimitWindowRepoStubForAdmin{}
		svc := &adminServiceImpl{userRepo: &userRepoStub{user: &User{ID: 7}}, userGroupRateLimitRepo: repo}

		_, err := svc.ListUserGroupRateLimitWindows(context.Background(), 0)
		require.Error(t, err)
		require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
		require.Zero(t, repo.listByUserID)
	})

	t.Run("propagates user not found", func(t *testing.T) {
		repo := &userGroupRateLimitWindowRepoStubForAdmin{}
		svc := &adminServiceImpl{userRepo: &userRepoStub{getErr: ErrUserNotFound}, userGroupRateLimitRepo: repo}

		_, err := svc.ListUserGroupRateLimitWindows(context.Background(), 99)
		require.ErrorIs(t, err, ErrUserNotFound)
		require.Zero(t, repo.listByUserID)
	})

	t.Run("returns service unavailable when repo missing", func(t *testing.T) {
		svc := &adminServiceImpl{userRepo: &userRepoStub{user: &User{ID: 7}}}

		_, err := svc.ListUserGroupRateLimitWindows(context.Background(), 7)
		require.Error(t, err)
		require.Equal(t, http.StatusServiceUnavailable, infraerrors.Code(err))
		require.Equal(t, "GROUP_RATE_LIMIT_REPOSITORY_UNAVAILABLE", infraerrors.Reason(err))
	})
}

func TestAdminService_ResetUserGroupRateLimitWindow(t *testing.T) {
	t.Run("validates user then resets window", func(t *testing.T) {
		repo := &userGroupRateLimitWindowRepoStubForAdmin{
			resetRecord: &UserGroupRateLimitWindowRecord{UserID: 7, GroupID: 2, GroupName: "pro"},
		}
		svc := &adminServiceImpl{
			userRepo:               &userRepoStub{user: &User{ID: 7}},
			userGroupRateLimitRepo: repo,
		}

		got, err := svc.ResetUserGroupRateLimitWindow(context.Background(), 7, 2)
		require.NoError(t, err)
		require.Equal(t, int64(7), repo.resetUserID)
		require.Equal(t, int64(2), repo.resetGroupID)
		require.Equal(t, int64(2), got.GroupID)
	})

	t.Run("rejects invalid ids before repo call", func(t *testing.T) {
		repo := &userGroupRateLimitWindowRepoStubForAdmin{}
		svc := &adminServiceImpl{userRepo: &userRepoStub{user: &User{ID: 7}}, userGroupRateLimitRepo: repo}

		_, err := svc.ResetUserGroupRateLimitWindow(context.Background(), 0, 2)
		require.Error(t, err)
		require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))

		_, err = svc.ResetUserGroupRateLimitWindow(context.Background(), 7, 0)
		require.Error(t, err)
		require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
		require.Zero(t, repo.resetUserID)
		require.Zero(t, repo.resetGroupID)
	})

	t.Run("propagates user and repo errors", func(t *testing.T) {
		repo := &userGroupRateLimitWindowRepoStubForAdmin{}
		svc := &adminServiceImpl{userRepo: &userRepoStub{getErr: ErrUserNotFound}, userGroupRateLimitRepo: repo}

		_, err := svc.ResetUserGroupRateLimitWindow(context.Background(), 99, 2)
		require.ErrorIs(t, err, ErrUserNotFound)
		require.Zero(t, repo.resetUserID)

		repoErr := errors.New("reset failed")
		repo = &userGroupRateLimitWindowRepoStubForAdmin{resetErr: repoErr}
		svc = &adminServiceImpl{userRepo: &userRepoStub{user: &User{ID: 7}}, userGroupRateLimitRepo: repo}

		_, err = svc.ResetUserGroupRateLimitWindow(context.Background(), 7, 2)
		require.ErrorIs(t, err, repoErr)
	})
}
