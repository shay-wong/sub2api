package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func init() { gin.SetMode(gin.TestMode) }

// ──────────────────────────────────────────────────────────
// NormalizeInboundEndpoint
// ──────────────────────────────────────────────────────────

func TestNormalizeInboundEndpoint(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		// Direct canonical paths.
		{"/v1/messages", EndpointMessages},
		{"/v1/chat/completions", EndpointChatCompletions},
		{"/v1/embeddings", EndpointEmbeddings},
		{"/v1/responses", EndpointResponses},
		{"/v1/responses/compact", EndpointResponsesCompact},
		{"/v1/responses/compact/detail", EndpointResponsesCompact},
		{"/v1/images/generations", EndpointImagesGenerations},
		{"/v1/images/edits", EndpointImagesEdits},
		{"/v1/videos/generations", EndpointVideosGenerations},
		{"/v1/videos/req_123", EndpointVideos},
		{"/v1beta/models", EndpointGeminiModels},

		// Prefixed paths (antigravity, openai) — root Responses.
		{"/antigravity/v1/messages", EndpointMessages},
		{"/openai/v1/responses", EndpointResponses},
		{"/openai/v1/images/generations", EndpointImagesGenerations},
		{"/openai/v1/images/edits", EndpointImagesEdits},
		{"/antigravity/v1beta/models/gemini:generateContent", EndpointGeminiModels},

		// Prefixed paths — "/responses/compact" is its OWN distinct
		// inbound endpoint, not folded into the root Responses endpoint.
		{"/openai/v1/responses/compact", EndpointResponsesCompact},
		{"/openai/v1/responses/compact/detail", EndpointResponsesCompact},

		// Bare top-level alias route "/responses" — root vs. compact.
		{"/responses", EndpointResponses},
		{"/responses/compact", EndpointResponsesCompact},
		{"/responses/compact/detail", EndpointResponsesCompact},

		// Bare Codex direct alias route — root vs. compact.
		{"/backend-api/codex/responses", EndpointResponses},
		{"/backend-api/codex/responses/compact", EndpointResponsesCompact},
		{"/backend-api/codex/responses/compact/detail", EndpointResponsesCompact},

		// Must NOT generalize to arbitrary paths merely ending in
		// "/responses" (or "/responses/compact") that are unrelated to
		// the two known bare alias roots, unless they already carry a
		// supported "/v1/responses..." prefix form.
		{"/foo/responses", "/foo/responses"},
		{"/foo/responses/compact", "/foo/responses/compact"},

		// Unknown path is returned as-is.
		{"/v1/embeddings", "/v1/embeddings"},
		{"", ""},
		{"  /v1/messages  ", EndpointMessages},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			require.Equal(t, tt.want, NormalizeInboundEndpoint(tt.path))
		})
	}
}

// ──────────────────────────────────────────────────────────
// DeriveUpstreamEndpoint
// ──────────────────────────────────────────────────────────

func TestDeriveUpstreamEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		inbound  string
		rawPath  string
		platform string
		want     string
	}{
		// Anthropic.
		{"anthropic messages", EndpointMessages, "/v1/messages", service.PlatformAnthropic, EndpointMessages},

		// Gemini.
		{"gemini models", EndpointGeminiModels, "/v1beta/models/gemini:gen", service.PlatformGemini, EndpointGeminiModels},

		// OpenAI — root Responses.
		{"openai responses root", EndpointResponses, "/v1/responses", service.PlatformOpenAI, EndpointResponses},

		// OpenAI — compact, raw path carries the derivable "/compact"
		// (or nested) suffix, which must be preserved on the upstream
		// endpoint.
		{"openai responses compact", EndpointResponsesCompact, "/openai/v1/responses/compact", service.PlatformOpenAI, "/v1/responses/compact"},
		{"openai responses nested", EndpointResponsesCompact, "/openai/v1/responses/compact/detail", service.PlatformOpenAI, "/v1/responses/compact/detail"},
		{"openai bare responses compact", EndpointResponsesCompact, "/responses/compact", service.PlatformOpenAI, "/v1/responses/compact"},
		{"openai bare responses compact detail", EndpointResponsesCompact, "/responses/compact/detail", service.PlatformOpenAI, "/v1/responses/compact/detail"},
		{"openai codex direct responses compact", EndpointResponsesCompact, "/backend-api/codex/responses/compact", service.PlatformOpenAI, "/v1/responses/compact"},
		{"openai codex direct responses compact detail", EndpointResponsesCompact, "/backend-api/codex/responses/compact/detail", service.PlatformOpenAI, "/v1/responses/compact/detail"},

		// OpenAI — bare root alias routes normalize to root Responses.
		{"openai bare responses", EndpointResponses, "/responses", service.PlatformOpenAI, EndpointResponses},
		{"openai codex direct responses", EndpointResponses, "/backend-api/codex/responses", service.PlatformOpenAI, EndpointResponses},

		// OpenAI — inbound is already the canonical compact endpoint but
		// the raw path carries no derivable "/responses..." suffix (e.g.
		// it was already normalized upstream). Must not silently fall
		// back to the root Responses endpoint.
		{"openai responses compact inbound only, unrelated raw path", EndpointResponsesCompact, "/v1/messages", service.PlatformOpenAI, EndpointResponsesCompact},

		{"openai from messages", EndpointMessages, "/v1/messages", service.PlatformOpenAI, EndpointResponses},
		{"openai from completions", EndpointChatCompletions, "/v1/chat/completions", service.PlatformOpenAI, EndpointResponses},
		{"openai embeddings", EndpointEmbeddings, "/v1/embeddings", service.PlatformOpenAI, EndpointEmbeddings},
		{"openai image generations", EndpointImagesGenerations, "/v1/images/generations", service.PlatformOpenAI, EndpointImagesGenerations},
		{"openai image edits", EndpointImagesEdits, "/openai/v1/images/edits", service.PlatformOpenAI, EndpointImagesEdits},
		{"grok video generations", EndpointVideosGenerations, "/v1/videos/generations", service.PlatformGrok, EndpointVideosGenerations},
		{"grok video status", EndpointVideos, "/videos/req_123", service.PlatformGrok, EndpointVideos},

		// Antigravity — uses inbound to pick Claude vs Gemini upstream.
		{"antigravity claude", EndpointMessages, "/antigravity/v1/messages", service.PlatformAntigravity, EndpointMessages},
		{"antigravity gemini", EndpointGeminiModels, "/antigravity/v1beta/models", service.PlatformAntigravity, EndpointGeminiModels},

		// Unknown platform — passthrough.
		{"unknown platform", "/v1/embeddings", "/v1/embeddings", "unknown", "/v1/embeddings"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, DeriveUpstreamEndpoint(tt.inbound, tt.rawPath, tt.platform))
		})
	}
}

// ──────────────────────────────────────────────────────────
// responsesSubpathSuffix
// ──────────────────────────────────────────────────────────

func TestResponsesSubpathSuffix(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"/v1/responses", ""},
		{"/v1/responses/", ""},
		{"/v1/responses/compact", "/compact"},
		{"/openai/v1/responses/compact/detail", "/compact/detail"},
		{"/responses", ""},
		{"/responses/compact", "/compact"},
		{"/responses/compact/detail", "/compact/detail"},
		{"/backend-api/codex/responses", ""},
		{"/backend-api/codex/responses/compact", "/compact"},
		{"/backend-api/codex/responses/compact/detail", "/compact/detail"},
		{"/v1/messages", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			require.Equal(t, tt.want, responsesSubpathSuffix(tt.raw))
		})
	}
}

// ──────────────────────────────────────────────────────────
// InboundEndpointMiddleware + context helpers
// ──────────────────────────────────────────────────────────

func TestInboundEndpointMiddleware(t *testing.T) {
	router := gin.New()
	router.Use(InboundEndpointMiddleware())

	var captured string
	router.POST("/v1/messages", func(c *gin.Context) {
		captured = GetInboundEndpoint(c)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, EndpointMessages, captured)
}

func TestGetInboundEndpoint_FallbackWithoutMiddleware(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/antigravity/v1/messages", nil)

	// Middleware did not run — fallback to normalizing c.Request.URL.Path.
	got := GetInboundEndpoint(c)
	require.Equal(t, EndpointMessages, got)
}

// TestInboundEndpointMiddleware_WildcardRoutes verifies that, when a
// gateway route is registered with a Gin wildcard pattern (e.g.
// "/v1/responses/*subpath"), InboundEndpointMiddleware normalizes based
// on the concrete request path (c.Request.URL.Path) rather than the
// route pattern (c.FullPath()). Using c.FullPath() here would collapse
// every request under the wildcard — including "/v1/responses/compact"
// — down to the literal pattern string, which never matches the
// "compact" alias detection and would incorrectly normalize to the root
// Responses endpoint.
func TestInboundEndpointMiddleware_WildcardRoutes(t *testing.T) {
	tests := []struct {
		name        string
		routePath   string
		requestPath string
		want        string
	}{
		{
			name:        "v1 responses wildcard route, compact request",
			routePath:   "/v1/responses/*subpath",
			requestPath: "/v1/responses/compact",
			want:        EndpointResponsesCompact,
		},
		{
			name:        "bare responses wildcard route, compact request",
			routePath:   "/responses/*subpath",
			requestPath: "/responses/compact",
			want:        EndpointResponsesCompact,
		},
		{
			name:        "codex direct wildcard route, compact request",
			routePath:   "/backend-api/codex/responses/*subpath",
			requestPath: "/backend-api/codex/responses/compact",
			want:        EndpointResponsesCompact,
		},
		{
			name:        "v1 responses wildcard route, non-compact subpath request",
			routePath:   "/v1/responses/*subpath",
			requestPath: "/v1/responses/foo",
			want:        EndpointResponses,
		},
		{
			name:        "bare responses wildcard route, non-compact subpath request",
			routePath:   "/responses/*subpath",
			requestPath: "/responses/foo",
			want:        EndpointResponses,
		},
		{
			name:        "codex direct wildcard route, non-compact subpath request",
			routePath:   "/backend-api/codex/responses/*subpath",
			requestPath: "/backend-api/codex/responses/foo",
			want:        EndpointResponses,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(InboundEndpointMiddleware())

			var captured string
			router.POST(tt.routePath, func(c *gin.Context) {
				captured = GetInboundEndpoint(c)
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodPost, tt.requestPath, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, tt.want, captured)
		})
	}
}

// TestInboundEndpointMiddleware_GeminiWildcardRoute verifies that a Gemini
// wildcard route (e.g. "/v1beta/models/*modelAction", used to capture the
// ":generateContent"-style action suffix embedded in the path) is normalized
// to EndpointGeminiModels via InboundEndpointMiddleware, using the same real
// Gin routing path as TestInboundEndpointMiddleware_WildcardRoutes above.
func TestInboundEndpointMiddleware_GeminiWildcardRoute(t *testing.T) {
	router := gin.New()
	router.Use(InboundEndpointMiddleware())

	var captured string
	router.POST("/v1beta/models/*modelAction", func(c *gin.Context) {
		captured = GetInboundEndpoint(c)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, EndpointGeminiModels, captured)
}

// TestGetInboundEndpoint_FallbackWildcardRouteWithoutMiddleware verifies
// that when InboundEndpointMiddleware did NOT run (so no value is stored
// in gin.Context), the GetInboundEndpoint fallback path still prefers
// c.Request.URL.Path over c.FullPath(). This guards against the fallback
// regressing to prefer c.FullPath() again, which would misnormalize
// concrete requests matched by a wildcard route pattern (e.g.
// "/v1/responses/*subpath" matching "/v1/responses/compact") down to
// the root Responses endpoint.
func TestGetInboundEndpoint_FallbackWildcardRouteWithoutMiddleware(t *testing.T) {
	router := gin.New()
	// Deliberately do NOT register InboundEndpointMiddleware.

	var captured string
	router.POST("/v1/responses/*subpath", func(c *gin.Context) {
		// Sanity check: FullPath returns the route pattern, not the
		// concrete request path, when a wildcard route matches.
		require.Equal(t, "/v1/responses/*subpath", c.FullPath())
		captured = GetInboundEndpoint(c)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, EndpointResponsesCompact, captured)
}

func TestGetUpstreamEndpoint_FullFlow(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses/compact", nil)

	// Simulate middleware.
	c.Set(ctxKeyInboundEndpoint, NormalizeInboundEndpoint(c.Request.URL.Path))

	got := GetUpstreamEndpoint(c, service.PlatformOpenAI)
	require.Equal(t, "/v1/responses/compact", got)
}

func TestEffectiveGroupRateLimitGroupID_SelectionWinsOverAPIKeyGroup(t *testing.T) {
	apiKeyGroupID := int64(10)
	selectionGroupID := int64(20)

	got := EffectiveGroupRateLimitGroupID(
		&service.AccountSelectionResult{GroupID: &selectionGroupID},
		&service.APIKey{GroupID: &apiKeyGroupID},
	)

	require.NotNil(t, got)
	require.Equal(t, selectionGroupID, *got)
}

func TestEffectiveGroupRateLimitGroupID_FallsBackToAPIKeyGroup(t *testing.T) {
	apiKeyGroupID := int64(10)

	got := EffectiveGroupRateLimitGroupID(nil, &service.APIKey{GroupID: &apiKeyGroupID})

	require.NotNil(t, got)
	require.Equal(t, apiKeyGroupID, *got)
}

func TestEffectiveGroupRateLimitGroupID_ReturnsNilWhenNoGroup(t *testing.T) {
	require.Nil(t, EffectiveGroupRateLimitGroupID(nil, nil))
	require.Nil(t, EffectiveGroupRateLimitGroupID(&service.AccountSelectionResult{}, &service.APIKey{}))
}

func TestEffectiveGroupRateLimitGroup_SelectionGroupWins(t *testing.T) {
	apiKeyGroup := &service.Group{ID: 10, RateLimit5h: 100}
	selectionGroup := &service.Group{ID: 20, RateLimit5h: 10}

	got := EffectiveGroupRateLimitGroup(
		&service.AccountSelectionResult{GroupID: &selectionGroup.ID, Group: selectionGroup},
		&service.APIKey{GroupID: &apiKeyGroup.ID, Group: apiKeyGroup},
	)

	require.Same(t, selectionGroup, got)
}

func TestEffectiveGroupRateLimitGroup_DoesNotUseAPIKeyGroupForDifferentSelectionID(t *testing.T) {
	apiKeyGroupID := int64(10)
	selectionGroupID := int64(20)

	got := EffectiveGroupRateLimitGroup(
		&service.AccountSelectionResult{GroupID: &selectionGroupID},
		&service.APIKey{GroupID: &apiKeyGroupID, Group: &service.Group{ID: apiKeyGroupID}},
	)

	require.Nil(t, got)
}

func TestEffectiveQuotaPlatform_SelectionGroupWinsOverAPIKeyGroup(t *testing.T) {
	apiKeyGroupID := int64(10)
	selectionGroupID := int64(20)

	got := EffectiveQuotaPlatform(
		context.Background(),
		&service.AccountSelectionResult{
			GroupID: &selectionGroupID,
			Group:   &service.Group{ID: selectionGroupID, Platform: service.PlatformGemini},
		},
		&service.APIKey{
			GroupID: &apiKeyGroupID,
			Group:   &service.Group{ID: apiKeyGroupID, Platform: service.PlatformAnthropic},
		},
	)

	require.Equal(t, service.PlatformGemini, got)
}

func TestEffectiveQuotaPlatform_ForcePlatformWinsOverSelectionGroup(t *testing.T) {
	selectionGroupID := int64(20)
	ctx := context.WithValue(context.Background(), ctxkey.ForcePlatform, service.PlatformAntigravity)

	got := EffectiveQuotaPlatform(
		ctx,
		&service.AccountSelectionResult{
			GroupID: &selectionGroupID,
			Group:   &service.Group{ID: selectionGroupID, Platform: service.PlatformGemini},
		},
		&service.APIKey{},
	)

	require.Equal(t, service.PlatformAntigravity, got)
}

func TestHandleSelectedOpenAIPreflight_UsesSelectedGroupForImagePermission(t *testing.T) {
	apiKeyGroupID := int64(10)
	selectedGroupID := int64(20)
	released := false
	var gotStatus int
	var gotCode string
	var gotMessage string

	subscription, failed := handleSelectedOpenAIPreflight(
		context.Background(),
		nil,
		nil,
		nil,
		&service.AccountSelectionResult{
			GroupID: &selectedGroupID,
			Group:   &service.Group{ID: selectedGroupID, Hydrated: true, Platform: service.PlatformOpenAI, Status: service.StatusActive},
		},
		&service.APIKey{
			GroupID: &apiKeyGroupID,
			Group: &service.Group{
				ID:                   apiKeyGroupID,
				Hydrated:             true,
				Platform:             service.PlatformOpenAI,
				Status:               service.StatusActive,
				AllowImageGeneration: true,
			},
		},
		nil,
		true,
		func() { released = true },
		nil,
		func(status int, code, message string) {
			gotStatus = status
			gotCode = code
			gotMessage = message
		},
	)

	require.True(t, failed)
	require.Nil(t, subscription)
	require.True(t, released)
	require.Equal(t, http.StatusForbidden, gotStatus)
	require.Equal(t, "permission_error", gotCode)
	require.Equal(t, service.ImageGenerationPermissionMessage(), gotMessage)
}

func TestHandleSelectedOpenAIPreflight_ResolvesSelectedSubscription(t *testing.T) {
	apiKeyGroupID := int64(10)
	selectedGroupID := int64(20)
	userID := int64(7)
	originalSub := &service.UserSubscription{ID: 100, UserID: userID, GroupID: apiKeyGroupID}
	selectedSub := &service.UserSubscription{ID: 200, UserID: userID, GroupID: selectedGroupID}
	subRepo := &selectedOpenAIPreflightSubRepoStub{sub: selectedSub}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	billingSvc := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, nil, cfg, nil)
	defer billingSvc.Stop()
	gatewaySvc := service.NewOpenAIGatewayService(
		nil,
		nil,
		nil,
		nil,
		subRepo,
		nil,
		nil,
		cfg,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	subscription, failed := handleSelectedOpenAIPreflight(
		context.Background(),
		nil,
		billingSvc,
		gatewaySvc,
		&service.AccountSelectionResult{
			GroupID: &selectedGroupID,
			Group: &service.Group{
				ID:               selectedGroupID,
				Hydrated:         true,
				Platform:         service.PlatformOpenAI,
				Status:           service.StatusActive,
				SubscriptionType: service.SubscriptionTypeSubscription,
			},
		},
		&service.APIKey{
			User:    &service.User{ID: userID},
			GroupID: &apiKeyGroupID,
			Group: &service.Group{
				ID:               apiKeyGroupID,
				Hydrated:         true,
				Platform:         service.PlatformOpenAI,
				Status:           service.StatusActive,
				SubscriptionType: service.SubscriptionTypeSubscription,
			},
		},
		originalSub,
		false,
		nil,
		nil,
		nil,
	)

	require.False(t, failed)
	require.Same(t, selectedSub, subscription)
	require.Equal(t, 1, subRepo.calls)
	require.Equal(t, userID, subRepo.lastUserID)
	require.Equal(t, selectedGroupID, subRepo.lastGroupID)
}

func TestHandleSelectedOpenAIPreflight_UsesSelectedGroupPlatformForQuota(t *testing.T) {
	apiKeyGroupID := int64(10)
	selectedGroupID := int64(20)
	userID := int64(7)
	quotaCache := &selectedOpenAIPreflightQuotaCache{}
	cfg := &config.Config{}
	cfg.Billing.UserPlatformQuotaCacheTTLSeconds = 60
	billingSvc := service.NewBillingCacheService(quotaCache, nil, nil, nil, nil, nil, nil, cfg, &selectedOpenAIPreflightQuotaRepoStub{})
	defer billingSvc.Stop()

	released := false
	var gotStatus int
	var gotCode string
	subscription, failed := handleSelectedOpenAIPreflight(
		context.Background(),
		nil,
		billingSvc,
		nil,
		&service.AccountSelectionResult{
			GroupID: &selectedGroupID,
			Group: &service.Group{
				ID:               selectedGroupID,
				Hydrated:         true,
				Platform:         service.PlatformGemini,
				Status:           service.StatusActive,
				SubscriptionType: service.SubscriptionTypeStandard,
			},
		},
		&service.APIKey{
			User:    &service.User{ID: userID},
			GroupID: &apiKeyGroupID,
			Group: &service.Group{
				ID:               apiKeyGroupID,
				Hydrated:         true,
				Platform:         service.PlatformOpenAI,
				Status:           service.StatusActive,
				SubscriptionType: service.SubscriptionTypeStandard,
			},
		},
		nil,
		false,
		func() { released = true },
		nil,
		func(status int, code, _ string) {
			gotStatus = status
			gotCode = code
		},
	)

	require.True(t, failed)
	require.Nil(t, subscription)
	require.True(t, released)
	require.Equal(t, http.StatusTooManyRequests, gotStatus)
	require.Equal(t, "rate_limit_exceeded", gotCode)
	require.Equal(t, []string{service.PlatformGemini}, quotaCache.platforms)
}

type selectedOpenAIPreflightSubRepoStub struct {
	service.UserSubscriptionRepository
	sub         *service.UserSubscription
	calls       int
	lastUserID  int64
	lastGroupID int64
}

func (s *selectedOpenAIPreflightSubRepoStub) GetActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	s.calls++
	s.lastUserID = userID
	s.lastGroupID = groupID
	return s.sub, nil
}

type selectedOpenAIPreflightQuotaCache struct {
	service.BillingCache
	platforms []string
}

func (s *selectedOpenAIPreflightQuotaCache) GetUserBalance(context.Context, int64) (float64, error) {
	return 100, nil
}

func (s *selectedOpenAIPreflightQuotaCache) GetUserPlatformQuotaCache(_ context.Context, _ int64, platform string) (*service.UserPlatformQuotaCacheEntry, bool, error) {
	s.platforms = append(s.platforms, platform)
	now := time.Now().UTC()
	if platform == service.PlatformGemini {
		zero := 0.0
		return &service.UserPlatformQuotaCacheEntry{
			DailyLimitUSD:    &zero,
			DailyWindowStart: &now,
			SchemaVersion:    service.UserPlatformQuotaCacheSchemaV1,
		}, true, nil
	}
	return &service.UserPlatformQuotaCacheEntry{
		DailyWindowStart: &now,
		SchemaVersion:    service.UserPlatformQuotaCacheSchemaV1,
	}, true, nil
}

func (s *selectedOpenAIPreflightQuotaCache) SetUserPlatformQuotaCache(context.Context, int64, string, *service.UserPlatformQuotaCacheEntry, time.Duration) error {
	return nil
}

type selectedOpenAIPreflightQuotaRepoStub struct {
	service.UserPlatformQuotaRepository
}
