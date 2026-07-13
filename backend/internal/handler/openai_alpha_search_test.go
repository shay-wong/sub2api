package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type alphaSearchCyberCache struct {
	service.GatewayCache
	blocked map[string]bool
}

func (c *alphaSearchCyberCache) SetCyberSessionBlocked(_ context.Context, key string, _ time.Duration) error {
	if c.blocked == nil {
		c.blocked = make(map[string]bool)
	}
	c.blocked[key] = true
	return nil
}

func (c *alphaSearchCyberCache) IsCyberSessionBlocked(_ context.Context, key string) (bool, error) {
	return c.blocked[key], nil
}

func newAlphaSearchHandlerTestContext(body string) (*gin.Context, *httptest.ResponseRecorder, *service.APIKey) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/alpha/search", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(91)
	group := &service.Group{ID: groupID, Name: "OpenAI", Platform: service.PlatformOpenAI}
	apiKey := &service.APIKey{ID: 701, Name: "alpha-search-test", GroupID: &groupID, Group: group}
	c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 12, Concurrency: 1})
	return c, recorder, apiKey
}

func newAlphaSearchPreflightHandler(gatewayService *service.OpenAIGatewayService, moderationService *service.ContentModerationService) *OpenAIGatewayHandler {
	return &OpenAIGatewayHandler{
		gatewayService:           gatewayService,
		billingCacheService:      &service.BillingCacheService{},
		apiKeyService:            &service.APIKeyService{},
		contentModerationService: moderationService,
		concurrencyHelper: NewConcurrencyHelper(
			service.NewConcurrencyService(&concurrencyCacheMock{}),
			SSEPingFormatNone,
			time.Second,
		),
	}
}

func TestAlphaSearchRunsContentModerationBeforeScheduling(t *testing.T) {
	moderationServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Contains(t, string(payload), "blocked alpha query")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"category_scores":{"sexual":0.9}}]}`))
	}))
	defer moderationServer.Close()

	moderationCfg := &service.ContentModerationConfig{
		Enabled:      true,
		Mode:         service.ContentModerationModePreBlock,
		BaseURL:      moderationServer.URL,
		Model:        "omni-moderation-latest",
		APIKeys:      []string{"sk-test"},
		SampleRate:   100,
		AllGroups:    true,
		BlockMessage: "alpha search blocked",
	}
	rawCfg, err := json.Marshal(moderationCfg)
	require.NoError(t, err)
	moderationService := service.NewContentModerationService(
		&contentModerationHandlerSettingRepo{values: map[string]string{
			service.SettingKeyRiskControlEnabled:      "true",
			service.SettingKeyContentModerationConfig: string(rawCfg),
		}},
		&contentModerationHandlerTestRepo{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	body := `{"id":"alpha-session","model":"gpt-5.1","commands":{"search_query":[{"q":"blocked alpha query"}]}}`
	c, recorder, _ := newAlphaSearchHandlerTestContext(body)
	h := newAlphaSearchPreflightHandler(&service.OpenAIGatewayService{}, moderationService)

	h.AlphaSearch(c)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "content_policy_violation")
	require.Contains(t, recorder.Body.String(), "alpha search blocked")
}

func TestAlphaSearchUsesRequestIDForCyberSessionPreflight(t *testing.T) {
	body := `{"id":"alpha-session","model":"gpt-5.1","commands":{"search_query":[{"q":"allowed query"}]}}`
	c, recorder, apiKey := newAlphaSearchHandlerTestContext(body)
	blockKey := service.CyberSessionBlockKeyWithFallback(apiKey.ID, c, []byte(body), "alpha-session")
	require.NotEmpty(t, blockKey)

	cache := &alphaSearchCyberCache{blocked: map[string]bool{blockKey: true}}
	settingService := service.NewSettingService(
		&contentModerationHandlerSettingRepo{values: map[string]string{
			service.SettingKeyCyberSessionBlockEnabled:    "true",
			service.SettingKeyCyberSessionBlockTTLSeconds: "60",
		}},
		&config.Config{},
	)
	gatewayService := service.NewOpenAIGatewayService(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		cache,
		&config.Config{},
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
		settingService,
		nil,
		nil,
	)
	h := newAlphaSearchPreflightHandler(gatewayService, nil)

	h.AlphaSearch(c)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "session_blocked_by_cyber_policy")
}
