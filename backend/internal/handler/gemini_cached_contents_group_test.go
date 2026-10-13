//go:build unit

package handler

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func cachedBillingFallback(t *testing.T, balance float64) (*cachedBillingEnv, *service.Group) {
	t.Helper()
	env := newCachedBillingEnv(t, balance, 900)
	selected := &service.Group{ID: 43, Hydrated: true, Platform: service.PlatformGemini, Status: service.StatusActive, RateMultiplier: 2}
	env.groups.groups[selected.ID] = selected
	env.group.ClaudeCodeOnly = true
	env.group.FallbackGroupID = &selected.ID
	env.accounts[900].AccountGroups = []service.AccountGroup{{AccountID: 900, GroupID: selected.ID}}
	env.upstream.createBody = cachedBillingCreateBody(time.Now().Add(24 * time.Hour))
	return env, selected
}

func TestGeminiCachedContentsCreate_FallbackBillingKeepsResourceOwner(t *testing.T) {
	env, selected := cachedBillingFallback(t, 1000)
	rec := env.create()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	env.waitForUsage(t, 1)
	log := env.usage.first()
	require.Equal(t, selected.ID, *log.GroupID)
	require.Equal(t, env.apiKey.ID, log.APIKeyID)
	require.InDelta(t, log.TotalCost*2, log.ActualCost, 1e-9)
	require.Equal(t, env.group.ID, *env.apiKey.GroupID, "billing must not mutate owner key")
	for _, record := range env.repo.records {
		require.Equal(t, env.group.ID, record.GroupID)
		require.Equal(t, selected.ID, record.BillingGroupID)
		require.Equal(t, env.apiKey.ID, record.APIKeyID)
		get := env.serve(http.MethodGet, "/v1beta/cachedContents/"+record.PublicID, "", gin.Params{{Key: "cacheID", Value: record.PublicID}}, env.h.GeminiCachedContentsGet)
		require.Equal(t, http.StatusOK, get.Code)
	}
}

func TestGeminiCachedContentsCreate_FallbackUnaffordableStorageBillsSelectedInput(t *testing.T) {
	env, selected := cachedBillingFallback(t, 5)
	rec := env.create()
	require.NotEqual(t, http.StatusOK, rec.Code)
	env.waitForUsage(t, 1)
	log := env.usage.first()
	require.Equal(t, selected.ID, *log.GroupID)
	require.InDelta(t, cachedBillingTokens*cachedBillingInputPrice*2, log.ActualCost, 1e-9)
	require.Empty(t, env.repo.records)
}

func TestGeminiCachedContentsPatch_UsesStoredBillingGroupAndCurrentPrice(t *testing.T) {
	env, selected := cachedBillingFallback(t, 1000)
	env.seed(time.Now().Add(time.Hour))
	record := env.repo.records["abc"]
	record.BillingGroupID = selected.ID
	env.repo.records[record.PublicID].BillingGroupID = selected.ID
	selected.RateMultiplier = 3
	env.group.FallbackGroupID = nil // later routing changes do not move an existing resource's bill
	rec := env.patch()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	env.waitForUsage(t, 1)
	log := env.usage.first()
	require.Equal(t, selected.ID, *log.GroupID)
	require.InDelta(t, log.TotalCost*3, log.ActualCost, 1e-9)
	require.Equal(t, env.group.ID, env.repo.records[record.PublicID].GroupID)
}

func TestGeminiCachedContentsPatch_MissingBillingGroupRejectsExtensionOnly(t *testing.T) {
	env := newCachedBillingEnv(t, 1000, 900)
	env.seed(time.Now().Add(time.Hour))
	record := env.repo.records["abc"]
	env.repo.records[record.PublicID].BillingGroupID = 999
	rec := env.patch()
	require.NotEqual(t, http.StatusOK, rec.Code)
	require.Empty(t, env.upstream.calls)
	require.Empty(t, env.usage.totals())
	env.repo.records[record.PublicID].ExpireTime = time.Now().Add(time.Hour)
	shorten := env.serve(http.MethodPatch, "/v1beta/cachedContents/abc?updateMask=ttl", `{"ttl":"60s"}`, gin.Params{{Key: "cacheID", Value: "abc"}}, env.h.GeminiCachedContentsPatch)
	require.Equal(t, http.StatusOK, shorten.Code, shorten.Body.String())
	require.Empty(t, env.usage.totals())
}

func TestGeminiCachedContentsPatch_SubscriptionUsesSavedBillingGroup(t *testing.T) {
	env, selected := cachedBillingFallback(t, 0)
	env.seed(time.Now().Add(time.Hour))
	env.repo.records["abc"].BillingGroupID = selected.ID
	selected.SubscriptionType = service.SubscriptionTypeSubscription
	limit := 1000.0
	selected.DailyLimitUSD = &limit
	enableCachedBillingReservations(t, env, &cachedBillingReservationCache{balance: 0,
		sub: &service.SubscriptionCacheData{Status: service.SubscriptionStatusActive, ExpiresAt: time.Now().Add(time.Hour)}})
	sub := &service.UserSubscription{ID: 9, UserID: env.apiKey.UserID, GroupID: selected.ID, Status: service.SubscriptionStatusActive, ExpiresAt: time.Now().Add(time.Hour)}
	// The existing matching subscription is reused; the owner group is a balance group.
	key, actual, err := env.h.geminiCachedContentRecordBillingKey(context.Background(), env.apiKey, env.repo.records["abc"], sub)
	require.NoError(t, err)
	require.Same(t, sub, actual)
	require.Equal(t, selected.ID, *key.GroupID)
	rec := env.serve(http.MethodPatch, "/v1beta/cachedContents/abc?updateMask=ttl", `{"ttl":"86400s"}`,
		gin.Params{{Key: "cacheID", Value: "abc"}}, func(c *gin.Context) {
			c.Set(string(middleware.ContextKeySubscription), sub)
			env.h.GeminiCachedContentsPatch(c)
		})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	env.waitForUsage(t, 1)
	require.Equal(t, selected.ID, *env.usage.first().GroupID)
	require.Equal(t, sub.ID, *env.usage.first().SubscriptionID)
}

func TestGeminiCachedContentsCreate_FallbackResolvesSubscriptionAndRejectsItsLimit(t *testing.T) {
	env, selected := cachedBillingFallback(t, 0)
	selected.SubscriptionType = service.SubscriptionTypeSubscription
	limit := 10.0
	selected.DailyLimitUSD = &limit
	sub := &service.UserSubscription{ID: 9, UserID: env.apiKey.UserID, GroupID: selected.ID,
		Status: service.SubscriptionStatusActive, ExpiresAt: time.Now().Add(time.Hour)}
	env.subs.subs[selected.ID] = sub
	cache := &cachedBillingReservationCache{balance: 0,
		sub: &service.SubscriptionCacheData{Status: service.SubscriptionStatusActive, ExpiresAt: sub.ExpiresAt}}
	billing := service.NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{}, nil)
	t.Cleanup(billing.Stop)
	env.h.billingCacheService = billing
	rec := env.create()
	require.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
	require.Equal(t, 1, env.upstream.methodCalls(http.MethodPost), "zero owner balance must not block a funded subscription")
	require.Equal(t, 1, env.upstream.methodCalls(http.MethodDelete))
	env.waitForUsage(t, 1)
	require.Equal(t, sub.ID, env.subs.billedID)
	require.Equal(t, selected.ID, *env.usage.first().GroupID)
	require.Equal(t, sub.ID, *env.usage.first().SubscriptionID)
}

func TestGeminiCachedContentsReference_UsesStoredGroupAfterRoutingChanges(t *testing.T) {
	for _, action := range []string{"generateContent", "countTokens"} {
		t.Run(action, func(t *testing.T) {
			env, selected := cachedBillingFallback(t, 1000)
			env.seed(time.Now().Add(time.Hour))
			env.repo.records["abc"].BillingGroupID = selected.ID
			env.group.FallbackGroupID = nil
			env.upstream.createBody = `{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":1,"totalTokenCount":11}}`
			if action == "countTokens" {
				env.upstream.createBody = `{"totalTokens":10}`
			}
			rec := env.serve(http.MethodPost, "/v1beta/models/gemini-2.5-pro:"+action,
				`{"cachedContent":"cachedContents/abc","contents":[{"role":"user","parts":[{"text":"hello"}]}]}`,
				gin.Params{{Key: "modelAction", Value: "gemini-2.5-pro:" + action}}, env.h.GeminiV1BetaModels)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.Equal(t, 1, env.upstream.methodCalls(http.MethodPost))
			if action == "generateContent" {
				env.waitForUsage(t, 1)
				require.Equal(t, selected.ID, *env.usage.first().GroupID)
			}
		})
	}
}

func TestGeminiCachedContentsReference_RejectsDisabledBillingGroup(t *testing.T) {
	env, selected := cachedBillingFallback(t, 1000)
	env.seed(time.Now().Add(time.Hour))
	env.repo.records["abc"].BillingGroupID = selected.ID
	selected.Status = service.StatusDisabled
	rec := env.serve(http.MethodPost, "/v1beta/models/gemini-2.5-pro:countTokens",
		`{"cachedContent":"cachedContents/abc","contents":[]}`,
		gin.Params{{Key: "modelAction", Value: "gemini-2.5-pro:countTokens"}}, env.h.GeminiV1BetaModels)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
	require.Empty(t, env.upstream.calls)
}
