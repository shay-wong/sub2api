package handler

import (
	"context"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ──────────────────────────────────────────────────────────
// Canonical inbound / upstream endpoint paths.
// All normalization and derivation reference this single set
// of constants — add new paths HERE when a new API surface
// is introduced.
// ──────────────────────────────────────────────────────────

const (
	EndpointMessages          = "/v1/messages"
	EndpointChatCompletions   = "/v1/chat/completions"
	EndpointEmbeddings        = "/v1/embeddings"
	EndpointResponses         = "/v1/responses"
	EndpointImagesGenerations = "/v1/images/generations"
	EndpointImagesEdits       = "/v1/images/edits"
	EndpointGeminiModels      = "/v1beta/models"
)

// gin.Context keys used by the middleware and helpers below.
const (
	ctxKeyInboundEndpoint = "_gateway_inbound_endpoint"
)

// ──────────────────────────────────────────────────────────
// Normalization functions
// ──────────────────────────────────────────────────────────

// NormalizeInboundEndpoint maps a raw request path (which may carry
// prefixes like /antigravity, /openai) to its canonical form.
//
//	"/antigravity/v1/messages"   → "/v1/messages"
//	"/v1/chat/completions"       → "/v1/chat/completions"
//	"/openai/v1/responses/foo"   → "/v1/responses"
//	"/v1beta/models/gemini:gen"  → "/v1beta/models"
func NormalizeInboundEndpoint(path string) string {
	path = strings.TrimSpace(path)
	switch {
	case strings.Contains(path, EndpointEmbeddings):
		return EndpointEmbeddings
	case strings.Contains(path, EndpointChatCompletions):
		return EndpointChatCompletions
	case strings.Contains(path, EndpointMessages):
		return EndpointMessages
	case strings.Contains(path, EndpointImagesGenerations) || strings.Contains(path, "/images/generations"):
		return EndpointImagesGenerations
	case strings.Contains(path, EndpointImagesEdits) || strings.Contains(path, "/images/edits"):
		return EndpointImagesEdits
	case strings.Contains(path, EndpointResponses):
		return EndpointResponses
	case strings.Contains(path, EndpointGeminiModels):
		return EndpointGeminiModels
	default:
		return path
	}
}

// DeriveUpstreamEndpoint determines the upstream endpoint from the
// account platform and the normalized inbound endpoint.
//
// Platform-specific rules:
//   - OpenAI always forwards to /v1/responses (with optional subpath
//     such as /v1/responses/compact preserved from the raw URL).
//   - Anthropic  → /v1/messages
//   - Gemini     → /v1beta/models
//   - Antigravity → /v1/messages (Claude) or gemini (Gemini)
//   - Antigravity routes may target either Claude or Gemini, so the
//     inbound endpoint is used to distinguish.
func DeriveUpstreamEndpoint(inbound, rawRequestPath, platform string) string {
	inbound = strings.TrimSpace(inbound)

	switch platform {
	case service.PlatformOpenAI, service.PlatformGrok:
		if inbound == EndpointEmbeddings || inbound == EndpointImagesGenerations || inbound == EndpointImagesEdits {
			return inbound
		}
		// OpenAI forwards everything to the Responses API.
		// Preserve subresource suffix (e.g. /v1/responses/compact).
		if suffix := responsesSubpathSuffix(rawRequestPath); suffix != "" {
			return EndpointResponses + suffix
		}
		return EndpointResponses

	case service.PlatformAnthropic:
		return EndpointMessages

	case service.PlatformGemini:
		return EndpointGeminiModels

	case service.PlatformAntigravity:
		// Antigravity accounts serve both Claude and Gemini.
		if inbound == EndpointGeminiModels {
			return EndpointGeminiModels
		}
		return EndpointMessages
	}

	// Unknown platform — fall back to inbound.
	return inbound
}

// responsesSubpathSuffix extracts the part after "/responses" in a raw
// request path, e.g. "/openai/v1/responses/compact" → "/compact".
// Returns "" when there is no meaningful suffix.
func responsesSubpathSuffix(rawPath string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(rawPath), "/")
	idx := strings.LastIndex(trimmed, "/responses")
	if idx < 0 {
		return ""
	}
	suffix := trimmed[idx+len("/responses"):]
	if suffix == "" || suffix == "/" {
		return ""
	}
	if !strings.HasPrefix(suffix, "/") {
		return ""
	}
	return suffix
}

// ──────────────────────────────────────────────────────────
// Middleware
// ──────────────────────────────────────────────────────────

// InboundEndpointMiddleware normalizes the request path and stores the
// canonical inbound endpoint in gin.Context so that every handler in
// the chain can read it via GetInboundEndpoint.
//
// Apply this middleware to all gateway route groups.
func InboundEndpointMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.FullPath()
		if path == "" && c.Request != nil && c.Request.URL != nil {
			path = c.Request.URL.Path
		}
		c.Set(ctxKeyInboundEndpoint, NormalizeInboundEndpoint(path))
		c.Next()
	}
}

// ──────────────────────────────────────────────────────────
// Context helpers — used by handlers before building
// RecordUsageInput / RecordUsageLongContextInput.
// ──────────────────────────────────────────────────────────

// GetInboundEndpoint returns the canonical inbound endpoint stored by
// InboundEndpointMiddleware. If the middleware did not run (e.g. in
// tests), it falls back to normalizing c.FullPath() on the fly.
func GetInboundEndpoint(c *gin.Context) string {
	if v, ok := c.Get(ctxKeyInboundEndpoint); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	// Fallback: normalize on the fly.
	path := ""
	if c != nil {
		path = c.FullPath()
		if path == "" && c.Request != nil && c.Request.URL != nil {
			path = c.Request.URL.Path
		}
	}
	return NormalizeInboundEndpoint(path)
}

// GetUpstreamEndpoint derives the upstream endpoint from the context
// and the account platform. Handlers call this after scheduling an
// account, passing account.Platform.
func GetUpstreamEndpoint(c *gin.Context, platform string) string {
	inbound := GetInboundEndpoint(c)
	rawPath := ""
	if c != nil && c.Request != nil && c.Request.URL != nil {
		rawPath = c.Request.URL.Path
	}
	return DeriveUpstreamEndpoint(inbound, rawPath, platform)
}

// EffectiveGroupRateLimitGroupID returns the actual group that should receive
// post-usage group 5h limit increments. Selection wins because fallback routing
// may execute a request through a different group than the API key's default.
func EffectiveGroupRateLimitGroupID(selection *service.AccountSelectionResult, apiKey *service.APIKey) *int64 {
	if selection != nil && selection.GroupID != nil && *selection.GroupID > 0 {
		id := *selection.GroupID
		return &id
	}
	if apiKey != nil && apiKey.GroupID != nil && *apiKey.GroupID > 0 {
		id := *apiKey.GroupID
		return &id
	}
	return nil
}

// EffectiveGroupRateLimitGroup returns the group object to use for the
// selected-request 5h preflight. It intentionally returns nil when selection
// points at a different group but did not carry that group's snapshot; falling
// back to the API key group in that case would check the wrong window.
func EffectiveGroupRateLimitGroup(selection *service.AccountSelectionResult, apiKey *service.APIKey) *service.Group {
	if selection != nil && selection.Group != nil && selection.Group.ID > 0 {
		return selection.Group
	}
	if selection != nil && selection.GroupID != nil && *selection.GroupID > 0 {
		if apiKey != nil && apiKey.Group != nil && apiKey.Group.ID == *selection.GroupID {
			return apiKey.Group
		}
		return nil
	}
	if apiKey != nil {
		return apiKey.Group
	}
	return nil
}

func CheckEffectiveGroupRateLimit5h(ctx context.Context, billingCacheService *service.BillingCacheService, selection *service.AccountSelectionResult, apiKey *service.APIKey) error {
	if billingCacheService == nil || apiKey == nil {
		return nil
	}
	groupID := EffectiveGroupRateLimitGroupID(selection, apiKey)
	if groupID == nil || (apiKey.GroupID != nil && *apiKey.GroupID == *groupID) {
		return nil
	}
	return billingCacheService.CheckGroupRateLimit5h(ctx, apiKey.User, EffectiveGroupRateLimitGroup(selection, apiKey))
}

func handleEffectiveGroupRateLimit5h(
	ctx context.Context,
	c *gin.Context,
	billingCacheService *service.BillingCacheService,
	selection *service.AccountSelectionResult,
	apiKey *service.APIKey,
	release func(),
	logFailure func(error),
	respond func(status int, code, message string),
) bool {
	err := CheckEffectiveGroupRateLimit5h(ctx, billingCacheService, selection, apiKey)
	if err == nil {
		return false
	}
	if logFailure != nil {
		logFailure(err)
	}
	if release != nil {
		release()
	}
	status, code, message, retryAfter := billingErrorDetails(err)
	if retryAfter > 0 && c != nil {
		c.Header("Retry-After", strconv.Itoa(retryAfter))
	}
	if respond != nil {
		respond(status, code, message)
	}
	return true
}
