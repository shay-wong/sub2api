package handler

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type selectedGroupEstimator struct {
	groups []int64
}

func (e *selectedGroupEstimator) EstimateInflightReservation(_ context.Context, key *service.APIKey, _ service.InflightEstimateRequest) (float64, bool) {
	e.groups = append(e.groups, *key.GroupID)
	return key.Group.RateMultiplier, true
}

func TestSelectedInflightBalanceFollowsGroupAndPreservesBillingReferences(t *testing.T) {
	cache := newHandlerInflightCache(10)
	cfg := &config.Config{}
	cfg.Billing.InflightReservation = config.InflightReservationConfig{Enabled: true, TTLSeconds: 60}
	billing := service.NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	original := &service.Group{ID: 1, SubscriptionType: service.SubscriptionTypeSubscription}
	key := &service.APIKey{User: &service.User{ID: 5}, GroupID: &original.ID, Group: original}
	group := &service.Group{ID: 2, RateMultiplier: 2}
	selection := &service.AccountSelectionResult{GroupID: &group.ID, Group: group}
	state := &selectedInflightBalance{request: tokenInflightEstimate("gpt-6-astra", nil)}
	t.Cleanup(state.close)
	estimator := &selectedGroupEstimator{}
	ctx := context.Background()
	require.NoError(t, state.reserve(ctx, billing, estimator, selection, key, nil))
	require.Equal(t, []int64{2}, estimator.groups)
	require.Equal(t, float64(2), state.reservation.Amount())
	require.Same(t, original, key.Group, "API key snapshot must not be mutated")
	require.NoError(t, state.reserve(ctx, billing, estimator, selection, key, nil))
	require.Len(t, estimator.groups, 1, "same-group failover must reuse its reservation")
	require.Equal(t, 1, cache.count())

	oldCtx := service.WithInflightReservation(ctx, state.reservation)
	billingDone := state.reservation.Acquire()
	selection = &service.AccountSelectionResult{GroupID: &original.ID, Group: original}
	require.NoError(t, state.reserve(oldCtx, billing, estimator, selection, key, &service.UserSubscription{ID: 9}))
	require.Nil(t, state.reservation, "subscription selection must clear the old reservation")
	require.Nil(t, service.InflightReservationFromContext(service.WithInflightReservation(oldCtx, state.reservation)))
	require.Equal(t, 1, cache.count(), "queued billing owns the old reservation until settled")
	billingDone()
	require.Zero(t, cache.count())

	group = &service.Group{ID: 3, RateMultiplier: 3}
	selection = &service.AccountSelectionResult{GroupID: &group.ID, Group: group}
	require.NoError(t, state.reserve(oldCtx, billing, estimator, selection, key, nil))
	require.Equal(t, []int64{2, 3}, estimator.groups)
	require.Equal(t, float64(3), state.reservation.Amount())
	state.close()
	require.Zero(t, cache.count())
}
