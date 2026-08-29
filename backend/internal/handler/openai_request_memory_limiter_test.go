package handler

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIRequestMemoryLimiter_SerializesIncidentSizedRequests(t *testing.T) {
	limiter := newOpenAIRequestMemoryLimiter(256 << 20)
	first := &http.Request{ContentLength: 23_352_325}
	second := &http.Request{ContentLength: 38_707_514}

	releaseFirst, err := limiter.Acquire(context.Background(), first)
	require.NoError(t, err)
	require.NotNil(t, releaseFirst)

	acquired := make(chan func(), 1)
	go func() {
		release, acquireErr := limiter.Acquire(context.Background(), second)
		if acquireErr == nil {
			acquired <- release
		}
	}()

	select {
	case release := <-acquired:
		release()
		t.Fatal("second oversized request should wait for memory budget")
	case <-time.After(20 * time.Millisecond):
	}

	releaseFirst()
	select {
	case release := <-acquired:
		require.NotNil(t, release)
		release()
	case <-time.After(time.Second):
		t.Fatal("second oversized request did not resume")
	}
}

func TestOpenAIRequestMemoryLimiter_SmallRequestsBypassBudget(t *testing.T) {
	limiter := newOpenAIRequestMemoryLimiter(256 << 20)
	req := &http.Request{ContentLength: 1 << 20}

	release, err := limiter.Acquire(context.Background(), req)

	require.NoError(t, err)
	require.Nil(t, release)
}

func TestOpenAIRequestMemoryLimiter_CanceledWaitStops(t *testing.T) {
	limiter := newOpenAIRequestMemoryLimiter(256 << 20)
	large := &http.Request{ContentLength: 38_707_514}
	release, err := limiter.Acquire(context.Background(), large)
	require.NoError(t, err)
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	waitRelease, err := limiter.Acquire(ctx, large)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, waitRelease)
}

func TestOpenAIRequestMemoryLimiter_UnknownOrCompressedBodyUsesFullBudget(t *testing.T) {
	limiter := newOpenAIRequestMemoryLimiter(256 << 20)

	require.Equal(t, int64(256<<20), limiter.requestWeight(&http.Request{ContentLength: -1}))
	require.Equal(t, int64(256<<20), limiter.requestWeight(&http.Request{
		ContentLength: 1 << 20,
		Header:        http.Header{"Content-Encoding": []string{"gzip"}},
	}))
}
