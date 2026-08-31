//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type failingSubscriptionInvalidationCache struct {
	billingCacheWorkerStub
	invalidateErr error
	publishCalls  int
}

func (c *failingSubscriptionInvalidationCache) InvalidateSubscriptionCache(context.Context, int64, int64) error {
	return c.invalidateErr
}

func (c *failingSubscriptionInvalidationCache) PublishSubscriptionCacheInvalidation(context.Context, string) error {
	c.publishCalls++
	return nil
}

func (c *failingSubscriptionInvalidationCache) SubscribeSubscriptionCacheInvalidation(context.Context, func(string)) error {
	return nil
}

func TestInvalidateSubscriptionCaches_PublishesAfterDistributedInvalidationFailure(t *testing.T) {
	invalidateErr := errors.New("injected distributed invalidation failure")
	cache := &failingSubscriptionInvalidationCache{invalidateErr: invalidateErr}
	billingCacheSvc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{}, nil)
	t.Cleanup(billingCacheSvc.Stop)

	svc := &SubscriptionService{billingCacheService: billingCacheSvc}
	err := svc.invalidateSubscriptionCaches(10, 20)

	require.ErrorIs(t, err, invalidateErr)
	require.Equal(t, 1, cache.publishCalls, "分布式删除失败也必须广播跨实例 L1 失效")
}

type revokeCacheUserSubRepoStub struct {
	userSubRepoNoop

	sub            *UserSubscription
	deleted        bool
	getActiveCalls int
}

func (r *revokeCacheUserSubRepoStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id || r.deleted {
		return nil, ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func (r *revokeCacheUserSubRepoStub) GetByIDForUpdate(ctx context.Context, id int64) (*UserSubscription, error) {
	return r.GetByID(ctx, id)
}

func (r *revokeCacheUserSubRepoStub) ExtendExpiry(_ context.Context, id int64, expiresAt time.Time) error {
	if r.sub == nil || r.sub.ID != id || r.deleted {
		return ErrSubscriptionNotFound
	}
	r.sub.ExpiresAt = expiresAt
	return nil
}

func (r *revokeCacheUserSubRepoStub) Delete(_ context.Context, id int64) error {
	if r.sub == nil || r.sub.ID != id || r.deleted {
		return ErrSubscriptionNotFound
	}
	r.deleted = true
	return nil
}

func (r *revokeCacheUserSubRepoStub) GetActiveByUserIDAndGroupID(_ context.Context, userID, groupID int64) (*UserSubscription, error) {
	r.getActiveCalls++
	if r.deleted || r.sub == nil || r.sub.UserID != userID || r.sub.GroupID != groupID {
		return nil, ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func TestRevokeSubscription_InvalidatesL1CacheSynchronously(t *testing.T) {
	repo := &revokeCacheUserSubRepoStub{
		sub: &UserSubscription{
			ID:        1,
			UserID:    10,
			GroupID:   20,
			Status:    SubscriptionStatusActive,
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, &config.Config{
		SubscriptionCache: config.SubscriptionCacheConfig{
			L1Size:       16,
			L1TTLSeconds: 60,
		},
	})
	t.Cleanup(svc.Stop)

	_, err := svc.GetActiveSubscription(context.Background(), 10, 20)
	require.NoError(t, err)
	svc.subCacheL1.Wait()
	require.Equal(t, 1, repo.getActiveCalls)

	err = svc.RevokeSubscription(context.Background(), 1)
	require.NoError(t, err)

	_, err = svc.GetActiveSubscription(context.Background(), 10, 20)
	require.ErrorIs(t, err, ErrSubscriptionNotFound)
	require.Equal(t, 2, repo.getActiveCalls, "撤销后应回源确认订阅已不存在，不能命中旧 L1")
}

func TestRedeemSubscriptionInvalidatesReloadedL1AfterCommit(t *testing.T) {
	const (
		userID  = int64(10)
		groupID = int64(20)
	)
	repo := &revokeCacheUserSubRepoStub{
		sub: &UserSubscription{
			ID:        1,
			UserID:    userID,
			GroupID:   groupID,
			Status:    SubscriptionStatusActive,
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	subscriptionSvc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, &config.Config{
		SubscriptionCache: config.SubscriptionCacheConfig{
			L1Size:       16,
			L1TTLSeconds: 60,
		},
	})
	t.Cleanup(subscriptionSvc.Stop)

	_, err := subscriptionSvc.GetActiveSubscription(context.Background(), userID, groupID)
	require.NoError(t, err)
	subscriptionSvc.subCacheL1.Wait()
	require.Equal(t, 1, repo.getActiveCalls)

	// Model a reader repopulating the old entitlement after an early invalidation
	// but before the outer redeem transaction commits.
	subscriptionSvc.InvalidateSubCacheSync(userID, groupID)
	_, err = subscriptionSvc.GetActiveSubscription(context.Background(), userID, groupID)
	require.NoError(t, err)
	subscriptionSvc.subCacheL1.Wait()
	require.Equal(t, 2, repo.getActiveCalls)

	repo.deleted = true
	redeemSvc := &RedeemService{subscriptionService: subscriptionSvc}
	redeemGroupID := groupID
	redeemSvc.invalidateRedeemCaches(context.Background(), userID, &RedeemCode{
		Type:    RedeemTypeSubscription,
		GroupID: &redeemGroupID,
	})

	_, err = subscriptionSvc.GetActiveSubscription(context.Background(), userID, groupID)
	require.ErrorIs(t, err, ErrSubscriptionNotFound)
	require.Equal(t, 3, repo.getActiveCalls, "提交后必须再次清除并发回填的旧 L1 权益")
}

func TestExtendSubscriptionPublishesAfterDistributedInvalidationFailure(t *testing.T) {
	const (
		userID  = int64(10)
		groupID = int64(20)
	)
	repo := &revokeCacheUserSubRepoStub{sub: &UserSubscription{
		ID:        1,
		UserID:    userID,
		GroupID:   groupID,
		Status:    SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(48 * time.Hour),
	}}
	cache := &failingSubscriptionInvalidationCache{
		invalidateErr: errors.New("injected distributed invalidation failure"),
	}
	billingCacheSvc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{}, nil)
	t.Cleanup(billingCacheSvc.Stop)
	svc := NewSubscriptionService(groupRepoNoop{}, repo, billingCacheSvc, nil, &config.Config{
		SubscriptionCache: config.SubscriptionCacheConfig{
			L1Size:       16,
			L1TTLSeconds: 60,
		},
	})
	t.Cleanup(svc.Stop)

	_, err := svc.GetActiveSubscription(context.Background(), userID, groupID)
	require.NoError(t, err)
	svc.subCacheL1.Wait()
	require.Equal(t, 1, repo.getActiveCalls)

	_, err = svc.ExtendSubscription(context.Background(), 1, -1)
	require.NoError(t, err)
	require.Equal(t, 1, cache.publishCalls, "分布式删除失败也必须广播管理员订阅调整")

	_, err = svc.GetActiveSubscription(context.Background(), userID, groupID)
	require.NoError(t, err)
	require.Equal(t, 2, repo.getActiveCalls, "管理员订阅调整提交后必须同步清除本机 L1")
}

type restoreUserSubRepoStub struct {
	userSubRepoNoop

	sub            *UserSubscription
	existsActive   bool
	restoreCalls   int
	restoredStatus string
}

func (r *restoreUserSubRepoStub) GetByIDIncludeDeleted(_ context.Context, id int64) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func (r *restoreUserSubRepoStub) ExistsActiveByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	return r.existsActive, nil
}

func (r *restoreUserSubRepoStub) Restore(_ context.Context, id int64, restoredStatus string) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	r.restoreCalls++
	r.restoredStatus = restoredStatus
	cp := *r.sub
	cp.Status = restoredStatus
	cp.DeletedAt = nil
	r.sub = &cp
	return &cp, nil
}

func TestRestoreSubscription_ExpiredActiveRestoresAsExpired(t *testing.T) {
	deletedAt := time.Now().Add(-time.Hour)
	repo := &restoreUserSubRepoStub{
		sub: &UserSubscription{
			ID:        1,
			UserID:    10,
			GroupID:   20,
			Status:    SubscriptionStatusActive,
			ExpiresAt: time.Now().Add(-time.Minute),
			DeletedAt: &deletedAt,
		},
	}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	t.Cleanup(svc.Stop)

	restored, err := svc.RestoreSubscription(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, 1, repo.restoreCalls)
	require.Equal(t, SubscriptionStatusExpired, repo.restoredStatus)
	require.Equal(t, SubscriptionStatusExpired, restored.Status)
	require.Nil(t, restored.DeletedAt)
}

func TestRestoreSubscription_NotRevokedReturnsConflict(t *testing.T) {
	repo := &restoreUserSubRepoStub{
		sub: &UserSubscription{
			ID:        1,
			UserID:    10,
			GroupID:   20,
			Status:    SubscriptionStatusActive,
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	t.Cleanup(svc.Stop)

	_, err := svc.RestoreSubscription(context.Background(), 1)
	require.ErrorIs(t, err, ErrSubscriptionNotRevoked)
	require.Zero(t, repo.restoreCalls)
}

func TestRestoreSubscription_LiveSubscriptionConflict(t *testing.T) {
	deletedAt := time.Now().Add(-time.Hour)
	repo := &restoreUserSubRepoStub{
		existsActive: true,
		sub: &UserSubscription{
			ID:        1,
			UserID:    10,
			GroupID:   20,
			Status:    SubscriptionStatusExpired,
			ExpiresAt: time.Now().Add(-time.Hour),
			DeletedAt: &deletedAt,
		},
	}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	t.Cleanup(svc.Stop)

	_, err := svc.RestoreSubscription(context.Background(), 1)
	require.ErrorIs(t, err, ErrSubscriptionRestoreConflict)
	require.Zero(t, repo.restoreCalls)
}
