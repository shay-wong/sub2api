package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestGetStickySessionAccountID_FallbackToLegacyKey(t *testing.T) {
	beforeFallbackTotal, beforeFallbackHit, _ := openAIStickyCompatStats()

	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{
			"openai:legacy-hash": 42,
		},
	}
	svc := &OpenAIGatewayService{
		cache: cache,
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				OpenAIWS: config.GatewayOpenAIWSConfig{
					SessionHashReadOldFallback: true,
				},
			},
		},
	}

	ctx := withOpenAILegacySessionHash(context.Background(), "legacy-hash")
	accountID, err := svc.getStickySessionAccountID(ctx, nil, "new-hash")
	require.NoError(t, err)
	require.Equal(t, int64(42), accountID)

	afterFallbackTotal, afterFallbackHit, _ := openAIStickyCompatStats()
	require.Equal(t, beforeFallbackTotal+1, afterFallbackTotal)
	require.Equal(t, beforeFallbackHit+1, afterFallbackHit)
}

func TestSetStickySessionAccountID_DualWriteOldEnabled(t *testing.T) {
	_, _, beforeDualWriteTotal := openAIStickyCompatStats()

	cache := &stubGatewayCache{sessionBindings: map[string]int64{}}
	svc := &OpenAIGatewayService{
		cache: cache,
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				OpenAIWS: config.GatewayOpenAIWSConfig{
					SessionHashDualWriteOld: true,
				},
			},
		},
	}

	ctx := withOpenAILegacySessionHash(context.Background(), "legacy-hash")
	err := svc.setStickySessionAccountID(ctx, nil, "new-hash", 9, openaiStickySessionTTL)
	require.NoError(t, err)
	require.Equal(t, int64(9), cache.sessionBindings["openai:new-hash"])
	require.Equal(t, int64(9), cache.sessionBindings["openai:legacy-hash"])

	_, _, afterDualWriteTotal := openAIStickyCompatStats()
	require.Equal(t, beforeDualWriteTotal+1, afterDualWriteTotal)
}

func TestSetStickySessionAccountID_DualWriteOldDisabled(t *testing.T) {
	cache := &stubGatewayCache{sessionBindings: map[string]int64{}}
	svc := &OpenAIGatewayService{
		cache: cache,
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				OpenAIWS: config.GatewayOpenAIWSConfig{
					SessionHashDualWriteOld: false,
				},
			},
		},
	}

	ctx := withOpenAILegacySessionHash(context.Background(), "legacy-hash")
	err := svc.setStickySessionAccountID(ctx, nil, "new-hash", 9, openaiStickySessionTTL)
	require.NoError(t, err)
	require.Equal(t, int64(9), cache.sessionBindings["openai:new-hash"])
	_, exists := cache.sessionBindings["openai:legacy-hash"]
	require.False(t, exists)
}

// The reasoning history flag must outlive sticky-account deletion so an unavailable
// passthrough account cannot leak its encrypted history into a non-passthrough retry.
func TestOpenAIReasoningSourcePassthroughSurvivesStickyDeletion(t *testing.T) {
	cache := &stubGatewayCache{
		sessionBindings:          map[string]int64{},
		reasoningSourceBySession: map[string]bool{},
	}
	svc := &OpenAIGatewayService{cache: cache}
	ctx := context.Background()

	_, known, err := svc.GetOpenAIReasoningSourcePassthrough(ctx, 11, nil, "missing-session")
	require.NoError(t, err)
	require.False(t, known)

	require.NoError(t, svc.BindOpenAIReasoningSourcePassthrough(ctx, 11, nil, "session-hash", true))
	require.NoError(t, svc.deleteStickySessionAccountID(ctx, nil, "session-hash"))

	passthrough, known, err := svc.GetOpenAIReasoningSourcePassthrough(ctx, 11, nil, "session-hash")
	require.NoError(t, err)
	require.True(t, known)
	require.True(t, passthrough)
}

// A later compatible response does not remove passthrough reasoning from older
// turns that the client may resend as part of the full conversation history.
func TestOpenAIReasoningSourcePassthroughIsMonotonicAcrossTurns(t *testing.T) {
	cache := &stubGatewayCache{reasoningSourceBySession: map[string]bool{}}
	svc := &OpenAIGatewayService{cache: cache}
	ctx := context.Background()

	require.NoError(t, svc.BindOpenAIReasoningSourcePassthrough(ctx, 11, nil, "three-turn-session", true))
	require.NoError(t, svc.BindOpenAIReasoningSourcePassthrough(ctx, 11, nil, "three-turn-session", false))

	mayContainPassthrough, known, err := svc.GetOpenAIReasoningSourcePassthrough(ctx, 11, nil, "three-turn-session")
	require.NoError(t, err)
	require.True(t, known)
	require.True(t, mayContainPassthrough)

	require.NoError(t, svc.BindOpenAIReasoningSourcePassthrough(ctx, 11, nil, "compatible-only-session", false))
	mayContainPassthrough, known, err = svc.GetOpenAIReasoningSourcePassthrough(ctx, 11, nil, "compatible-only-session")
	require.NoError(t, err)
	require.True(t, known)
	require.False(t, mayContainPassthrough)
}

func TestSnapshotOpenAICompatibilityFallbackMetrics(t *testing.T) {
	before := SnapshotOpenAICompatibilityFallbackMetrics()

	ctx := context.WithValue(context.Background(), ctxkey.ThinkingEnabled, true)
	_, _ = ThinkingEnabledFromContext(ctx)

	after := SnapshotOpenAICompatibilityFallbackMetrics()
	require.GreaterOrEqual(t, after.MetadataLegacyFallbackTotal, before.MetadataLegacyFallbackTotal+1)
	require.GreaterOrEqual(t, after.MetadataLegacyFallbackThinkingEnabledTotal, before.MetadataLegacyFallbackThinkingEnabledTotal+1)
}
