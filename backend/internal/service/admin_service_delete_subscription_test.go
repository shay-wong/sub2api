//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type deleteUserSubscriptionRepoStub struct {
	UserSubscriptionRepository
	subs    []UserSubscription
	err     error
	userIDs []int64
}

func (r *deleteUserSubscriptionRepoStub) ListByUserID(_ context.Context, userID int64) ([]UserSubscription, error) {
	r.userIDs = append(r.userIDs, userID)
	return r.subs, r.err
}

type deleteUserCommitRepoStub struct {
	userRepoStub
	beforeDelete func()
	committed    bool
}

func (r *deleteUserCommitRepoStub) Delete(ctx context.Context, id int64) error {
	r.beforeDelete()
	if err := r.userRepoStub.Delete(ctx, id); err != nil {
		return err
	}
	dbent.TxFromContext(ctx).OnCommit(func(next dbent.Committer) dbent.Committer {
		return dbent.CommitFunc(func(ctx context.Context, tx *dbent.Tx) error {
			if err := next.Commit(ctx, tx); err != nil {
				return err
			}
			r.committed = true
			return nil
		})
	})
	return nil
}

type deleteUserSubscriptionCacheStub struct {
	billingCacheWorkerStub
	check         func(context.Context)
	invalidations []subscriptionInvalidateCall
	published     []string
	err           error
}

func (c *deleteUserSubscriptionCacheStub) InvalidateSubscriptionCache(ctx context.Context, userID, groupID int64) error {
	c.check(ctx)
	c.invalidations = append(c.invalidations, subscriptionInvalidateCall{userID: userID, groupID: groupID})
	return c.err
}

func (c *deleteUserSubscriptionCacheStub) PublishSubscriptionCacheInvalidation(ctx context.Context, key string) error {
	c.check(ctx)
	c.published = append(c.published, key)
	return nil
}

func (*deleteUserSubscriptionCacheStub) SubscribeSubscriptionCacheInvalidation(context.Context, func(string)) error {
	return nil
}

func TestAdminService_DeleteUser_InvalidatesSubscriptionsAfterCommit(t *testing.T) {
	for _, tc := range []struct {
		name       string
		deleteFail bool
		commitFail bool
		cacheFail  bool
	}{
		{name: "success"},
		{name: "delete failure", deleteFail: true},
		{name: "commit failure", commitFail: true},
		{name: "cache failure remains successful", cacheFail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, sqlDB)))
			t.Cleanup(func() { _ = client.Close() })
			injectedErr := errors.New("injected failure")
			userRepo := &deleteUserCommitRepoStub{userRepoStub: userRepoStub{user: &User{ID: 7, Role: RoleUser}}}
			subRepo := &deleteUserSubscriptionRepoStub{subs: []UserSubscription{
				{ID: 1, UserID: 7, GroupID: 20, Status: SubscriptionStatusActive},
				{ID: 2, UserID: 7, GroupID: 21, Status: SubscriptionStatusExpired},
				{ID: 3, UserID: 7, GroupID: 22, Status: SubscriptionStatusSuspended},
			}}
			cache := &deleteUserSubscriptionCacheStub{check: func(ctx context.Context) {
				require.True(t, userRepo.committed, "缓存必须在数据库提交之后失效")
				require.Nil(t, dbent.TxFromContext(ctx))
				require.NoError(t, ctx.Err())
			}}
			if tc.cacheFail {
				cache.err = injectedErr
			}
			billingSvc := &BillingCacheService{cache: cache}
			subscriptionSvc := NewSubscriptionService(nil, subRepo, billingSvc, client, &config.Config{
				SubscriptionCache: config.SubscriptionCacheConfig{L1Size: 1024, L1TTLSeconds: 60},
			})
			t.Cleanup(subscriptionSvc.Stop)
			for _, sub := range subRepo.subs {
				require.True(t, subscriptionSvc.subCacheL1.Set(subCacheKey(sub.UserID, sub.GroupID), sub, 1))
			}
			subscriptionSvc.subCacheL1.Wait()
			invalidator := &authCacheInvalidatorStub{}
			userRepo.beforeDelete = func() {
				require.Empty(t, invalidator.userIDs)
				for _, sub := range subRepo.subs {
					_, cached := subscriptionSvc.subCacheL1.Get(subCacheKey(sub.UserID, sub.GroupID))
					require.True(t, cached, "事务内不能提前清理缓存")
				}
			}
			svc := &adminServiceImpl{
				userRepo: userRepo, userSubRepo: subRepo, subscriptionService: subscriptionSvc,
				entClient: client, authCacheInvalidator: invalidator,
			}
			mock.ExpectBegin()
			if tc.deleteFail {
				userRepo.deleteErr = injectedErr
				mock.ExpectRollback()
			} else if tc.commitFail {
				mock.ExpectCommit().WillReturnError(injectedErr)
			} else {
				mock.ExpectCommit()
			}

			err = svc.DeleteUser(context.Background(), 7)
			failed := tc.deleteFail || tc.commitFail
			if failed {
				require.ErrorIs(t, err, injectedErr)
				require.Empty(t, invalidator.userIDs)
				require.Empty(t, cache.invalidations)
				require.Empty(t, cache.published)
			} else {
				require.NoError(t, err)
				require.Equal(t, []int64{7}, invalidator.userIDs)
				require.Equal(t, []subscriptionInvalidateCall{{userID: 7, groupID: 20}, {userID: 7, groupID: 21}, {userID: 7, groupID: 22}}, cache.invalidations)
				require.Equal(t, []string{"sub:7:20", "sub:7:21", "sub:7:22"}, cache.published)
			}
			for _, sub := range subRepo.subs {
				_, cached := subscriptionSvc.subCacheL1.Get(subCacheKey(sub.UserID, sub.GroupID))
				require.Equal(t, failed, cached, "失败保留缓存，成功提交后同步清理 L1")
			}
			require.Equal(t, []int64{7}, subRepo.userIDs)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAdminService_DeleteUser_SubscriptionLookupFailurePreservesUser(t *testing.T) {
	injectedErr := errors.New("subscription lookup failed")
	userRepo := &userRepoStub{user: &User{ID: 7, Role: RoleUser}}
	subRepo := &deleteUserSubscriptionRepoStub{err: injectedErr}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{userRepo: userRepo, userSubRepo: subRepo, authCacheInvalidator: invalidator}

	err := svc.DeleteUser(context.Background(), 7)

	require.ErrorIs(t, err, injectedErr)
	require.Empty(t, userRepo.deletedIDs)
	require.Empty(t, invalidator.userIDs)
}

func TestUserService_Delete_InvalidatesAuthOnlyAfterSuccess(t *testing.T) {
	for _, deleteErr := range []error{nil, errors.New("delete failed")} {
		repo := &userRepoStub{deleteErr: deleteErr}
		invalidator := &authCacheInvalidatorStub{}
		svc := NewUserService(repo, nil, invalidator, nil)
		err := svc.Delete(context.Background(), 7)
		if deleteErr != nil {
			require.ErrorIs(t, err, deleteErr)
			require.Empty(t, invalidator.userIDs)
		} else {
			require.NoError(t, err)
			require.Equal(t, []int64{7}, invalidator.userIDs)
		}
	}
}
