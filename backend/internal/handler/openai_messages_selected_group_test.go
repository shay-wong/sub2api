//go:build unit

package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type openAIMessagesSelectedGroupUpstream struct {
	service.HTTPUpstream
	accountIDs []int64
	path       string
	body       []byte
}

func (u *openAIMessagesSelectedGroupUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	u.accountIDs = append(u.accountIDs, accountID)
	u.path, u.body = req.URL.Path, body
	contentType := "text/event-stream"
	payload := "data: " + `{"type":"response.completed","response":{"id":"resp_selected_group","object":"response","model":"gpt-6.1-sol","status":"completed","output":[{"type":"message","id":"msg_selected_group","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}}` + "\n\ndata: [DONE]\n\n"
	if strings.HasSuffix(req.URL.Path, "/chat/completions") {
		contentType = "application/json"
		payload = `{"id":"chat_selected_group","model":"gpt-6.1-sol","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}`
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(payload)),
	}, nil
}

func TestOpenAIMessagesSelectedGroupReasoningPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const (
		primaryGroupID  = int64(9610)
		fallbackGroupID = int64(9611)
		accountID       = int64(9612)
	)
	for _, protocol := range []struct {
		name       string
		mode       openai_compat.ResponsesSupportMode
		path       string
		effortPath string
	}{
		{name: "responses", mode: openai_compat.ResponsesSupportModeForceResponses, path: "/responses", effortPath: "reasoning.effort"},
		{name: "chat", mode: openai_compat.ResponsesSupportModeForceChatCompletions, path: "/chat/completions", effortPath: "reasoning_effort"},
	} {
		for _, tc := range []struct {
			name             string
			primaryMappings  []service.ReasoningEffortMapping
			fallbackMappings []service.ReasoningEffortMapping
			preference       string
			prebindPolicy    bool
			wantStatus       int
			wantEffort       string
		}{
			{
				name:             "fallback synthesized none mapping wins",
				primaryMappings:  []service.ReasoningEffortMapping{{From: "none", To: "deny"}},
				fallbackMappings: []service.ReasoningEffortMapping{{From: "none", To: "low"}},
				preference:       `,"thinking":{"type":"disabled"}`,
				wantStatus:       http.StatusOK,
				wantEffort:       "low",
			},
			{
				name:             "fallback deny wins",
				primaryMappings:  []service.ReasoningEffortMapping{{From: "none", To: "low"}},
				fallbackMappings: []service.ReasoningEffortMapping{{From: "none", To: "deny"}},
				preference:       `,"thinking":{"type":"disabled"}`,
				wantStatus:       http.StatusForbidden,
			},
			{
				name:            "empty fallback clears stale deny",
				primaryMappings: []service.ReasoningEffortMapping{{From: "high", To: "deny"}},
				preference:      `,"output_config":{"effort":"high"}`,
				prebindPolicy:   true,
				wantStatus:      http.StatusOK,
				wantEffort:      "high",
			},
		} {
			t.Run(protocol.name+"/"+tc.name, func(t *testing.T) {
				fallbackID := fallbackGroupID
				primary := &service.Group{
					ID: primaryGroupID, Platform: service.PlatformOpenAI, Status: service.StatusActive, Hydrated: true,
					ClaudeCodeOnly: true, FallbackGroupID: &fallbackID, AllowMessagesDispatch: true,
					ReasoningEffortMappings: tc.primaryMappings,
				}
				fallback := &service.Group{
					ID: fallbackGroupID, Platform: service.PlatformOpenAI, Status: service.StatusActive, Hydrated: true,
					AllowMessagesDispatch: true, ReasoningEffortMappings: tc.fallbackMappings,
				}
				account := service.Account{
					ID: accountID, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
					Status: service.StatusActive, Schedulable: true, Concurrency: 1,
					GroupIDs: []int64{fallbackGroupID}, AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: fallbackGroupID}},
					Credentials: map[string]any{
						"api_key": "fixture-key", "base_url": "https://fixture.example/v1",
						"model_mapping": map[string]any{"gpt-6.1-sol": "gpt-6.1-sol"},
					},
					Extra: map[string]any{openai_compat.ExtraKeyResponsesMode: string(protocol.mode)},
				}
				accountRepo := openAIImagesFailoverAccountRepo{accounts: []service.Account{account}}
				schedulerCache := &groupScopedSchedulerCache{fakeSchedulerCache: &fakeSchedulerCache{accounts: []*service.Account{&account}}}
				groupRepo := &groupMapRepo{fakeGroupRepo: &fakeGroupRepo{}, groups: map[int64]*service.Group{primaryGroupID: primary, fallbackGroupID: fallback}}
				// Keep real group buckets while bypassing unrelated balance storage.
				snapshot := service.NewSchedulerSnapshotService(schedulerCache, nil, accountRepo, groupRepo, nil)
				cfg := &config.Config{RunMode: config.RunModeSimple}
				upstream := &openAIMessagesSelectedGroupUpstream{}
				usageLogs := make(chan *service.UsageLog, 1)
				gateway := service.NewOpenAIGatewayService(
					accountRepo, &openAIWSUsageHandlerUsageLogRepoStub{created: usageLogs}, nil, nil, nil, nil, nil,
					cfg, snapshot, nil, service.NewBillingService(cfg, nil), nil, nil, upstream,
					&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
				)
				billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
				t.Cleanup(billing.Stop)
				handler := NewOpenAIGatewayHandler(
					gateway, service.NewConcurrencyService(nil), billing,
					service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg,
				)
				groupID := primaryGroupID
				apiKey := &service.APIKey{
					ID: 9620, UserID: 9621, GroupID: &groupID, Group: primary, Status: service.StatusActive,
					User: &service.User{ID: 9621, Status: service.StatusActive, Balance: 100},
				}
				body := `{"model":"gpt-6.1-sol","max_tokens":1024,"messages":[{"role":"user","content":"Hello"}],"stream":false` + tc.preference + `}`
				ctx := context.WithValue(context.Background(), ctxkey.Group, primary)
				if tc.prebindPolicy {
					ctx = service.WithOpenAIReasoningEffortPolicy(ctx, "", primary.ReasoningEffortMappings, "")
				}
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body)).WithContext(ctx)
				c.Request.Header.Set("Content-Type", "application/json")
				c.Set(string(middleware.ContextKeyAPIKey), apiKey)
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID})

				handler.Messages(c)

				require.Equal(t, tc.wantStatus, recorder.Code, recorder.Body.String())
				selected, ok := c.Get(opsAccountIDKey)
				require.True(t, ok, "the real Messages scheduler must select an account")
				require.Equal(t, accountID, selected)
				require.Contains(t, schedulerCache.queriedGroupIDs(), fallbackGroupID)
				require.NotContains(t, schedulerCache.queriedGroupIDs(), primaryGroupID)
				if tc.wantStatus == http.StatusForbidden {
					require.Empty(t, upstream.accountIDs)
					require.Contains(t, recorder.Body.String(), "reasoning")
					require.Empty(t, usageLogs)
					return
				}
				require.Equal(t, []int64{accountID}, upstream.accountIDs)
				require.True(t, strings.HasSuffix(upstream.path, protocol.path), upstream.path)
				require.Equal(t, "gpt-6.1-sol", gjson.GetBytes(upstream.body, "model").String())
				require.Equal(t, tc.wantEffort, gjson.GetBytes(upstream.body, protocol.effortPath).String())
				require.Equal(t, "ok", gjson.GetBytes(recorder.Body.Bytes(), "content.0.text").String())
				select {
				case usage := <-usageLogs:
					require.Equal(t, fallbackGroupID, *usage.GroupID)
				default:
					t.Fatal("successful forwarding must record usage without a fixture panic")
				}
			})
		}
	}
}

func TestMessagesSelectedGroupPolicyClearsUnsupportedTargets(t *testing.T) {
	for _, tc := range []struct {
		name           string
		platform       string
		resolvedTarget string
		missingGroup   bool
	}{
		{name: "missing fallback snapshot", missingGroup: true},
		{name: "native CN", platform: service.PlatformKimi},
		{name: "native Grok", platform: service.PlatformGrok},
		{name: "composite CN", platform: service.PlatformComposite, resolvedTarget: service.PlatformKimi},
		{name: "composite Grok", platform: service.PlatformComposite, resolvedTarget: service.PlatformGrok},
	} {
		t.Run(tc.name, func(t *testing.T) {
			primary := &service.Group{
				ID: 9630, Platform: service.PlatformOpenAI,
				ReasoningEffortMappings: []service.ReasoningEffortMapping{{From: "high", To: "deny"}},
			}
			selectedID := int64(9631)
			selection := &service.AccountSelectionResult{GroupID: &selectedID}
			if !tc.missingGroup {
				selection.Group = &service.Group{ID: selectedID, Platform: tc.platform, ReasoningEffortMappings: primary.ReasoningEffortMappings}
			}
			ctx := service.WithOpenAIReasoningEffortPolicy(context.Background(), "", primary.ReasoningEffortMappings, "")
			if tc.resolvedTarget != "" {
				ctx = service.WithResolvedTargetPlatform(ctx, tc.resolvedTarget)
			}
			converted := []byte(`{"model":"gpt-6.1-sol","reasoning":{"effort":"high"}}`)
			_, _, err := service.ApplyOpenAIReasoningEffortPolicyFromContext(ctx, converted)
			require.Error(t, err, "fixture must begin with a denying policy")
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)

			bindOpenAIReasoningEffortPolicyForMessagesRequest(c, &service.APIKey{Group: primary}, selection, []byte(`{"output_config":{"effort":"high"}}`))

			got, changed, err := service.ApplyOpenAIReasoningEffortPolicyFromContext(c.Request.Context(), converted)
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, converted, got)
		})
	}
}
