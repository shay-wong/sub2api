package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
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
		{"/v1/images/generations", EndpointImagesGenerations},
		{"/v1/images/edits", EndpointImagesEdits},
		{"/v1/videos/generations", EndpointVideosGenerations},
		{"/v1/videos/req_123", EndpointVideos},
		{"/v1beta/models", EndpointGeminiModels},

		// Prefixed paths (antigravity, openai).
		{"/antigravity/v1/messages", EndpointMessages},
		{"/openai/v1/responses", EndpointResponses},
		{"/openai/v1/responses/compact", EndpointResponses},
		{"/openai/v1/images/generations", EndpointImagesGenerations},
		{"/openai/v1/images/edits", EndpointImagesEdits},
		{"/antigravity/v1beta/models/gemini:generateContent", EndpointGeminiModels},

		// Gin route patterns with wildcards.
		{"/v1beta/models/*modelAction", EndpointGeminiModels},
		{"/v1/responses/*subpath", EndpointResponses},

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

		// OpenAI — always /v1/responses.
		{"openai responses root", EndpointResponses, "/v1/responses", service.PlatformOpenAI, EndpointResponses},
		{"openai responses compact", EndpointResponses, "/openai/v1/responses/compact", service.PlatformOpenAI, "/v1/responses/compact"},
		{"openai responses nested", EndpointResponses, "/openai/v1/responses/compact/detail", service.PlatformOpenAI, "/v1/responses/compact/detail"},
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
