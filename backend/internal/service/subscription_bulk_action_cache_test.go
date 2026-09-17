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

func TestBulkSubscriptionAction_CacheFailurePreservesMutationSuccess(t *testing.T) {
	for _, action := range []string{"extend", "reset_quota", "revoke", "restore"} {
		t.Run(action, func(t *testing.T) {
			sub := &UserSubscription{
				ID: 1, UserID: 10, GroupID: 20,
				Status: SubscriptionStatusActive, ExpiresAt: time.Now().AddDate(0, 0, 30),
			}
			if action == "restore" {
				deletedAt := time.Now().Add(-time.Hour)
				sub.DeletedAt = &deletedAt
			}
			repo := &bulkActionSubscriptionRepo{subscriptions: map[int64]*UserSubscription{1: sub}}
			cache := &failingSubscriptionInvalidationCache{invalidateErr: errors.New("cache unavailable")}
			billingCache := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{}, nil)
			t.Cleanup(billingCache.Stop)
			svc := NewSubscriptionService(nil, repo, nil, nil, nil)
			svc.billingCacheService = billingCache
			t.Cleanup(svc.Stop)

			result, err := svc.BulkSubscriptionAction(context.Background(), &BulkSubscriptionActionInput{
				SubscriptionIDs: []int64{1}, Action: action, Days: 7, Daily: true,
			})
			require.NoError(t, err)
			require.Equal(t, 1, result.SuccessCount)
			require.Zero(t, result.FailedCount)
			require.Equal(t, []int64{1}, repo.mutations)
			require.Equal(t, 1, cache.publishCalls, "invalidate once after the mutation completes")
		})
	}
}
