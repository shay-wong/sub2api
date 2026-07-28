package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestGatewayCacheLiveCallIdentityAndController(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	cache, ok := NewGatewayCache(client).(service.LiveCallStore)
	require.True(t, ok)
	otherInstance, ok := NewGatewayCache(client).(service.LiveCallStore)
	require.True(t, ok)
	record := &service.LiveCallRecord{
		CallID:                "call_secret",
		CallHash:              HashLiveCallID("call_secret"),
		AccountID:             11,
		APIKeyID:              22,
		ProjectID:             169,
		SessionID:             "sess-live-cache",
		UserID:                33,
		GroupID:               44,
		LeaseID:               "lease",
		Model:                 "gpt-live-test",
		AttestationCiphertext: "encrypted-attestation",
		CreatedAt:             time.Now(),
		ExpiresAt:             time.Now().Add(time.Hour),
		Controller:            service.LiveControllerPending,
	}
	require.NoError(t, cache.SaveLiveCall(context.Background(), record, time.Hour))

	loaded, err := otherInstance.GetLiveCall(context.Background(), record.CallHash)
	require.NoError(t, err)
	require.Equal(t, record.CallID, loaded.CallID)
	require.Equal(t, record.AccountID, loaded.AccountID)
	require.Equal(t, record.ProjectID, loaded.ProjectID)
	require.Equal(t, record.SessionID, loaded.SessionID)
	require.Equal(t, record.AttestationCiphertext, loaded.AttestationCiphertext)
	require.True(t, loaded.FinishedAt.IsZero())
	firstFinishedAt := time.Now().Add(-time.Minute).Truncate(time.Millisecond)
	frozenAt, err := cache.FreezeLiveCallFinishedAt(context.Background(), record.CallHash, firstFinishedAt)
	require.NoError(t, err)
	require.Equal(t, firstFinishedAt, frozenAt)
	frozenAt, err = cache.FreezeLiveCallFinishedAt(context.Background(), record.CallHash, time.Now())
	require.NoError(t, err)
	require.Equal(t, firstFinishedAt, frozenAt)
	loaded, err = otherInstance.GetLiveCall(context.Background(), record.CallHash)
	require.NoError(t, err)
	require.Equal(t, firstFinishedAt, loaded.FinishedAt)
	require.NoError(t, client.Del(context.Background(), liveCallKey(record.CallHash)).Err())
	record.FinishedAt = firstFinishedAt
	require.NoError(t, cache.ScheduleLiveCallRecovery(
		context.Background(), record, time.Now().Add(-time.Second), 2*time.Hour,
	))
	loaded, err = otherInstance.GetLiveCall(context.Background(), record.CallHash)
	require.NoError(t, err)
	require.Equal(t, firstFinishedAt, loaded.FinishedAt)
	recoverable, err := cache.ListRecoverableLiveCalls(context.Background(), time.Now(), 10)
	require.NoError(t, err)
	require.Len(t, recoverable, 1)
	require.Equal(t, record.CallHash, recoverable[0].CallHash)
	require.NoError(t, cache.ScheduleLiveCallRecovery(context.Background(), record, time.Now().Add(-time.Second), 2*time.Hour))
	require.Equal(t, 2*time.Hour, client.TTL(context.Background(), liveCallKey(record.CallHash)).Val())
	recoverable, err = cache.ListRecoverableLiveCalls(context.Background(), time.Now(), 10)
	require.NoError(t, err)
	require.Len(t, recoverable, 1)
	require.Equal(t, record.CallHash, recoverable[0].CallHash)

	claimed, err := cache.ClaimLiveController(context.Background(), record.CallHash, service.LiveControllerObserver, "observer-1")
	require.NoError(t, err)
	require.True(t, claimed)
	claimed, err = cache.ClaimLiveController(context.Background(), record.CallHash, service.LiveControllerProxy, "proxy-1")
	require.NoError(t, err)
	require.True(t, claimed)
	controller, err := cache.GetLiveController(context.Background(), record.CallHash)
	require.NoError(t, err)
	require.Equal(t, service.LiveControllerProxy, controller)

	released, err := cache.ReleaseLiveController(context.Background(), record.CallHash, "proxy-1")
	require.NoError(t, err)
	require.True(t, released)
	closed, err := cache.MarkLiveCallClosed(context.Background(), record.CallHash, time.Hour)
	require.NoError(t, err)
	require.Equal(t, service.LiveCallCloseFirst, closed)
	closed, err = cache.MarkLiveCallClosed(context.Background(), record.CallHash, time.Hour)
	require.NoError(t, err)
	require.Equal(t, service.LiveCallCloseAlready, closed)
	closed, err = cache.MarkLiveCallClosed(context.Background(), HashLiveCallID("missing"), time.Hour)
	require.NoError(t, err)
	require.Equal(t, service.LiveCallCloseMissing, closed)
	recoverable, err = cache.ListRecoverableLiveCalls(context.Background(), time.Now().Add(time.Hour), 10)
	require.NoError(t, err)
	require.Empty(t, recoverable)
}

func TestGatewayCachePromotesLiveCreateIntentAtomically(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	cache, ok := NewGatewayCache(client).(service.LiveCallStore)
	require.True(t, ok)
	intent := &service.LiveCallRecord{
		CallID:      "pending:lease-1",
		CallHash:    HashLiveCallID("pending:lease-1"),
		Provisional: true,
		AccountID:   11,
		APIKeyID:    22,
		UserID:      33,
		LeaseID:     "lease-1",
		Model:       "gpt-live-test",
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(time.Hour),
		Controller:  service.LiveControllerPending,
	}
	require.NoError(t, cache.SaveLiveCall(context.Background(), intent, 25*time.Hour))
	loadedIntent, err := cache.GetLiveCall(context.Background(), intent.CallHash)
	require.NoError(t, err)
	require.True(t, loadedIntent.Provisional)

	record := *intent
	record.CallID = "call_secret"
	record.CallHash = HashLiveCallID(record.CallID)
	require.Error(t, cache.PromoteLiveCall(context.Background(), intent.CallHash, &record, 25*time.Hour))
	record.Provisional = false
	require.NoError(t, cache.PromoteLiveCall(context.Background(), intent.CallHash, &record, 25*time.Hour))

	_, err = cache.GetLiveCall(context.Background(), intent.CallHash)
	require.ErrorIs(t, err, service.ErrLiveCallNotFound)
	loaded, err := cache.GetLiveCall(context.Background(), record.CallHash)
	require.NoError(t, err)
	require.Equal(t, record.CallID, loaded.CallID)
	require.False(t, loaded.Provisional)
	recoverable, err := cache.ListRecoverableLiveCalls(context.Background(), record.ExpiresAt.Add(time.Second), 10)
	require.NoError(t, err)
	require.Len(t, recoverable, 1)
	require.Equal(t, record.CallHash, recoverable[0].CallHash)
}

// Once passthrough reasoning enters a session history, later compatible turns
// may refresh its TTL but cannot clear the marker while old turns can be resent.
func TestGatewayCacheOpenAIReasoningSourcePassthroughIsMonotonicAndProjectScoped(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	store, ok := NewGatewayCache(client).(service.OpenAIReasoningSourceStore)
	require.True(t, ok)
	ctx := context.Background()

	require.NoError(t, store.SetOpenAIReasoningSourcePassthrough(ctx, 11, 7, "session", false, time.Hour))
	mayContainPassthrough, found, err := store.GetOpenAIReasoningSourcePassthrough(ctx, 11, 7, "session")
	require.NoError(t, err)
	require.True(t, found)
	require.False(t, mayContainPassthrough)

	_, found, err = store.GetOpenAIReasoningSourcePassthrough(ctx, 12, 7, "session")
	require.NoError(t, err)
	require.False(t, found)
	require.NoError(t, store.SetOpenAIReasoningSourcePassthrough(ctx, 12, 7, "session", false, time.Hour))

	require.NoError(t, store.SetOpenAIReasoningSourcePassthrough(ctx, 11, 7, "session", true, time.Hour))
	redisServer.FastForward(30 * time.Minute)
	require.NoError(t, store.SetOpenAIReasoningSourcePassthrough(ctx, 11, 7, "session", false, time.Hour))
	mayContainPassthrough, found, err = store.GetOpenAIReasoningSourcePassthrough(ctx, 11, 7, "session")
	require.NoError(t, err)
	require.True(t, found)
	require.True(t, mayContainPassthrough)
	require.Equal(t, time.Hour, redisServer.TTL(openAIReasoningSourceKey(11, 7, "session")))
	otherProjectPassthrough, found, err := store.GetOpenAIReasoningSourcePassthrough(ctx, 12, 7, "session")
	require.NoError(t, err)
	require.True(t, found)
	require.False(t, otherProjectPassthrough)
}
