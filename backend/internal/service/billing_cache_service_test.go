package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type billingCacheWorkerStub struct {
	balanceUpdates      int64
	subscriptionUpdates int64
}

func (b *billingCacheWorkerStub) GetUserBalance(ctx context.Context, userID int64) (float64, error) {
	return 0, errors.New("not implemented")
}

func (b *billingCacheWorkerStub) SetUserBalance(ctx context.Context, userID int64, balance float64) error {
	atomic.AddInt64(&b.balanceUpdates, 1)
	return nil
}

func (b *billingCacheWorkerStub) DeductUserBalance(ctx context.Context, userID int64, amount float64) error {
	atomic.AddInt64(&b.balanceUpdates, 1)
	return nil
}

func (b *billingCacheWorkerStub) InvalidateUserBalance(ctx context.Context, userID int64) error {
	return nil
}

func (b *billingCacheWorkerStub) GetSubscriptionCache(ctx context.Context, userID, groupID int64) (*SubscriptionCacheData, error) {
	return nil, errors.New("not implemented")
}

func (b *billingCacheWorkerStub) SetSubscriptionCache(ctx context.Context, userID, groupID int64, data *SubscriptionCacheData) error {
	atomic.AddInt64(&b.subscriptionUpdates, 1)
	return nil
}

func (b *billingCacheWorkerStub) UpdateSubscriptionUsage(ctx context.Context, userID, groupID int64, cost float64) error {
	atomic.AddInt64(&b.subscriptionUpdates, 1)
	return nil
}

func (b *billingCacheWorkerStub) InvalidateSubscriptionCache(ctx context.Context, userID, groupID int64) error {
	return nil
}

func (b *billingCacheWorkerStub) GetAPIKeyRateLimit(ctx context.Context, keyID int64) (*APIKeyRateLimitCacheData, error) {
	return nil, errors.New("not implemented")
}

func (b *billingCacheWorkerStub) SetAPIKeyRateLimit(ctx context.Context, keyID int64, data *APIKeyRateLimitCacheData) error {
	return nil
}

func (b *billingCacheWorkerStub) UpdateAPIKeyRateLimitUsage(ctx context.Context, keyID int64, cost float64) error {
	return nil
}

func (b *billingCacheWorkerStub) InvalidateAPIKeyRateLimit(ctx context.Context, keyID int64) error {
	return nil
}

func (b *billingCacheWorkerStub) GetUserPlatformQuotaCache(ctx context.Context, userID int64, platform string) (*UserPlatformQuotaCacheEntry, bool, error) {
	return nil, false, nil
}

func (b *billingCacheWorkerStub) SetUserPlatformQuotaCache(ctx context.Context, userID int64, platform string, entry *UserPlatformQuotaCacheEntry, ttl time.Duration) error {
	return nil
}

func (b *billingCacheWorkerStub) DeleteUserPlatformQuotaCache(ctx context.Context, userID int64, platform string) error {
	return nil
}

func (b *billingCacheWorkerStub) IncrUserPlatformQuotaUsageCache(ctx context.Context, userID int64, platform string, cost float64, ttl time.Duration, markDirty bool) error {
	return nil
}

func (b *billingCacheWorkerStub) PopDirtyUserPlatformQuotaKeys(ctx context.Context, n int) ([]UserPlatformQuotaKey, error) {
	return nil, nil
}

func (b *billingCacheWorkerStub) ReaddDirtyUserPlatformQuotaKeys(ctx context.Context, keys []UserPlatformQuotaKey) error {
	return nil
}

func (b *billingCacheWorkerStub) BatchGetUserPlatformQuotaCache(ctx context.Context, keys []UserPlatformQuotaKey) ([]*UserPlatformQuotaCacheEntry, error) {
	return nil, nil
}

func TestBillingCacheServiceQueueHighLoad(t *testing.T) {
	cache := &billingCacheWorkerStub{}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, nil, &config.Config{}, nil)
	t.Cleanup(svc.Stop)

	start := time.Now()
	for i := 0; i < cacheWriteBufferSize*2; i++ {
		svc.QueueDeductBalance(1, 1)
	}
	require.Less(t, time.Since(start), 2*time.Second)

	svc.QueueUpdateSubscriptionUsage(1, 2, 1.5)

	require.Eventually(t, func() bool {
		return atomic.LoadInt64(&cache.balanceUpdates) > 0
	}, 2*time.Second, 10*time.Millisecond)

	require.Eventually(t, func() bool {
		return atomic.LoadInt64(&cache.subscriptionUpdates) > 0
	}, 2*time.Second, 10*time.Millisecond)
}

func TestBillingCacheServiceEnqueueAfterStopReturnsFalse(t *testing.T) {
	cache := &billingCacheWorkerStub{}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, nil, &config.Config{}, nil)
	svc.Stop()

	enqueued := svc.enqueueCacheWrite(cacheWriteTask{
		kind:   cacheWriteDeductBalance,
		userID: 1,
		amount: 1,
	})
	require.False(t, enqueued)
}

type groupRateLimitWindowRepoStub struct {
	calls int32
	rec   *UserGroupRateLimitWindowRecord
	err   error
}

func (s *groupRateLimitWindowRepoStub) Get(ctx context.Context, userID, groupID int64) (*UserGroupRateLimitWindowRecord, error) {
	atomic.AddInt32(&s.calls, 1)
	if s.err != nil {
		return nil, s.err
	}
	return s.rec, nil
}

func (s *groupRateLimitWindowRepoStub) ListByUser(ctx context.Context, userID int64) ([]UserGroupRateLimitWindowRecord, error) {
	return nil, nil
}

func (s *groupRateLimitWindowRepoStub) ListByGroup(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]UserGroupRateLimitWindowRecord, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (s *groupRateLimitWindowRepoStub) IncrementWithWindowReset(ctx context.Context, userID, groupID int64, cost float64, now time.Time) error {
	return nil
}

func (s *groupRateLimitWindowRepoStub) Reset(ctx context.Context, userID, groupID int64) (*UserGroupRateLimitWindowRecord, error) {
	return nil, nil
}

func TestBillingCacheService_CheckGroupRateLimit5h_SelectedGroupExceeded(t *testing.T) {
	windowStart := time.Now().Add(-time.Hour)
	repo := &groupRateLimitWindowRepoStub{
		rec: &UserGroupRateLimitWindowRecord{
			UserID:        1,
			GroupID:       20,
			Usage5hUSD:    10,
			Window5hStart: &windowStart,
		},
	}
	svc := NewBillingCacheService(nil, nil, nil, nil, nil, nil, repo, &config.Config{}, nil)
	t.Cleanup(svc.Stop)

	err := svc.CheckGroupRateLimit5h(context.Background(), &User{ID: 1}, &Group{ID: 20, RateLimit5h: 10})

	require.ErrorIs(t, err, ErrGroupRateLimit5hExceeded)
	require.EqualValues(t, 1, atomic.LoadInt32(&repo.calls))
}

func TestBillingCacheService_CheckGroupRateLimit5h_SkipsOriginalClaudeCodeOnlyFallbackGroup(t *testing.T) {
	fallbackGroupID := int64(20)
	windowStart := time.Now().Add(-time.Hour)
	repo := &groupRateLimitWindowRepoStub{
		rec: &UserGroupRateLimitWindowRecord{
			UserID:        1,
			GroupID:       10,
			Usage5hUSD:    10,
			Window5hStart: &windowStart,
		},
	}
	svc := NewBillingCacheService(nil, nil, nil, nil, nil, nil, repo, &config.Config{}, nil)
	t.Cleanup(svc.Stop)

	err := svc.CheckGroupRateLimit5h(context.Background(), &User{ID: 1}, &Group{
		ID:              10,
		RateLimit5h:     10,
		ClaudeCodeOnly:  true,
		FallbackGroupID: &fallbackGroupID,
	})

	require.NoError(t, err)
	require.EqualValues(t, 0, atomic.LoadInt32(&repo.calls))
}

func TestBillingCacheService_CheckGroupRateLimit5h_ForcedPlatformDoesNotSkipClaudeCodeOnlyGroup(t *testing.T) {
	fallbackGroupID := int64(20)
	windowStart := time.Now().Add(-time.Hour)
	repo := &groupRateLimitWindowRepoStub{
		rec: &UserGroupRateLimitWindowRecord{
			UserID:        1,
			GroupID:       10,
			Usage5hUSD:    10,
			Window5hStart: &windowStart,
		},
	}
	svc := NewBillingCacheService(nil, nil, nil, nil, nil, nil, repo, &config.Config{}, nil)
	t.Cleanup(svc.Stop)
	ctx := context.WithValue(context.Background(), ctxkey.ForcePlatform, PlatformAntigravity)

	err := svc.CheckGroupRateLimit5h(ctx, &User{ID: 1}, &Group{
		ID:              10,
		RateLimit5h:     10,
		ClaudeCodeOnly:  true,
		FallbackGroupID: &fallbackGroupID,
	})

	require.ErrorIs(t, err, ErrGroupRateLimit5hExceeded)
	require.EqualValues(t, 1, atomic.LoadInt32(&repo.calls))
}
