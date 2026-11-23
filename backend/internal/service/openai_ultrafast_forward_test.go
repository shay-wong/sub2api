//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestUltrafastForwardUsesEntitledTierAndBilling(t *testing.T) {
	for _, tc := range []struct {
		name, path, plan, requestedModel, mappedModel, wantTier string
		apiKey, groupForced                                     bool
	}{
		{name: "native Pro 200 downgrades", plan: "pro", requestedModel: "gpt-6.1-sol", wantTier: "priority"},
		{name: "passthrough Pro 200 downgrades", path: "passthrough", plan: "pro", requestedModel: "gpt-6.1-sol", wantTier: "priority"},
		{name: "chat Pro 200 downgrades", path: "chat", plan: "pro", requestedModel: "gpt-6.1-sol", wantTier: "priority"},
		{name: "Pro 500 retains Ultra", plan: "promax", requestedModel: "gpt-6.1-sol", wantTier: "ultrafast"},
		{name: "Pro 200 supports newer Sol", plan: "pro", requestedModel: "gpt-6.2-sol", wantTier: "ultrafast"},
		{name: "mapped final model downgrades", plan: "pro", requestedModel: "gpt-6.2-sol", mappedModel: "gpt-6.1-sol", wantTier: "priority"},
		{name: "mapped final model retains Ultra", plan: "pro", requestedModel: "gpt-6.1-sol", mappedModel: "gpt-6.2-sol", wantTier: "ultrafast"},
		{name: "group forced Pro 200 downgrades", plan: "pro", requestedModel: "gpt-6.1-sol", wantTier: "priority", groupForced: true},
		{name: "group forced Astra downgrades", plan: "pro", requestedModel: "gpt-6.2-astra", wantTier: "priority", groupForced: true},
		{name: "API key keeps explicit Ultra", plan: "pro", requestedModel: "gpt-6.1-sol", wantTier: "ultrafast", apiKey: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setup := newAstraOAuthSetup(t, tc.path == "passthrough")
			setup.account.Credentials["plan_type"] = tc.plan
			if tc.apiKey {
				setup.account.Type = AccountTypeAPIKey
				setup.account.Credentials["api_key"] = "sk-test"
				setup.account.Extra["openai_responses_supported"] = true
			}
			finalModel := tc.requestedModel
			if tc.mappedModel != "" {
				setup.account.Credentials["model_mapping"] = map[string]any{tc.requestedModel: tc.mappedModel}
				finalModel = tc.mappedModel
			}
			responseTier := "default"
			if tc.apiKey {
				responseTier = tc.wantTier
			}
			response := fmt.Sprintf(`{"id":"resp_entitled_tier","object":"response","status":"completed","model":%q,"service_tier":%q,"output":[],"usage":{"input_tokens":100,"output_tokens":50}}`, finalModel, responseTier)
			setup.upstream.resp = &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(codexCompletedSSE(response))),
			}
			ctx := context.Background()
			tierField := `,"service_tier":"ultrafast"`
			if tc.groupForced {
				ctx = openAIUltrafastGroupContext()
				tierField = ""
			}
			body := []byte(fmt.Sprintf(`{"model":%q%s,"input":"hello","stream":false}`, tc.requestedModel, tierField))
			var result *OpenAIForwardResult
			var err error
			if tc.path == "chat" {
				body = []byte(fmt.Sprintf(`{"model":%q,"service_tier":"ultrafast","messages":[{"role":"user","content":"hello"}],"stream":false}`, tc.requestedModel))
				result, err = setup.svc.ForwardAsChatCompletions(ctx, setup.c, setup.account, body, "", "")
			} else {
				result, err = setup.svc.Forward(ctx, setup.c, setup.account, body)
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, finalModel, gjson.GetBytes(setup.upstream.lastBody, "model").String())
			require.Equal(t, tc.wantTier, gjson.GetBytes(setup.upstream.lastBody, "service_tier").String())
			require.NotNil(t, result.ServiceTier)
			require.Equal(t, tc.wantTier, *result.ServiceTier)
			if tc.apiKey {
				require.Empty(t, setup.upstream.lastReq.Header.Get(openAICodexRoutingHintHeader))
			} else {
				require.Equal(t, "model="+finalModel+";tier="+tc.wantTier, setup.upstream.lastReq.Header.Get(openAICodexRoutingHintHeader))
			}

			assertUltrafastForwardBilling(t, setup.account, result, finalModel, tc.wantTier)
		})
	}
}

func TestUltrafastForwardCompactFallbackRechecksFinalModel(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, failedSSE := range []bool{false, true} {
			for _, stream := range []bool{false, true} {
				for _, direction := range []struct{ model, fallback, firstTier, finalTier string }{
					{"gpt-6.2-sol", "gpt-6.1-sol", "ultrafast", "priority"},
					{"gpt-6.1-sol", "gpt-6.2-sol", "priority", "ultrafast"},
				} {
					name := fmt.Sprintf("passthrough=%t/sse_failure=%t/stream=%t/%s", passthrough, failedSSE, stream, direction.model)
					t.Run(name, func(t *testing.T) {
						setup := newAstraOAuthSetup(t, passthrough)
						setup.account.Credentials["plan_type"] = "pro"
						setup.svc.cfg.Gateway.OpenAICompactModel = direction.fallback
						MarkOpenAINativeCompactionV2(setup.c)
						firstStatus, firstType := http.StatusBadRequest, "application/json"
						firstBody := `{"error":{"code":"context_length_exceeded","message":"context window exceeded"}}`
						if failedSSE {
							firstStatus, firstType = http.StatusOK, "text/event-stream"
							firstBody = "event: response.failed\ndata: " + `{"type":"response.failed","response":{"status":"failed","error":{"code":"context_length_exceeded","message":"context window exceeded"}}}` + "\n\n"
						}
						completed := fmt.Sprintf(`{"id":"resp_compact_tier","object":"response","status":"completed","model":%q,"service_tier":"default","output":[],"usage":{"input_tokens":100,"output_tokens":50}}`, direction.fallback)
						setup.upstream.responses = []*http.Response{
							{StatusCode: firstStatus, Header: http.Header{"Content-Type": []string{firstType}}, Body: io.NopCloser(strings.NewReader(firstBody))},
							{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(codexCompletedSSE(completed)))},
						}
						body := []byte(fmt.Sprintf(`{"model":%q,"service_tier":"ultrafast","stream":%t,"input":[{"type":"message","role":"user","content":"hello"},{"type":"compaction_trigger"}]}`, direction.model, stream))
						result, err := setup.svc.Forward(context.Background(), setup.c, setup.account, body)
						require.NoError(t, err)
						require.NotNil(t, result)
						require.Len(t, setup.upstream.bodies, 2)
						for i, expected := range []struct{ model, tier string }{{direction.model, direction.firstTier}, {direction.fallback, direction.finalTier}} {
							require.Equal(t, expected.model, gjson.GetBytes(setup.upstream.bodies[i], "model").String(), "attempt %d", i+1)
							require.Equal(t, expected.tier, gjson.GetBytes(setup.upstream.bodies[i], "service_tier").String(), "attempt %d", i+1)
							require.Equal(t, "model="+expected.model+";tier="+expected.tier, setup.upstream.requests[i].Header.Get(openAICodexRoutingHintHeader), "attempt %d", i+1)
						}
						require.NotNil(t, result.ServiceTier)
						require.Equal(t, direction.finalTier, *result.ServiceTier)
						require.Equal(t, direction.model, result.Model)
						require.Equal(t, direction.fallback, result.UpstreamModel)
						// Compact fallback preserves the request's billing model while using the final outbound tier.
						assertUltrafastForwardBilling(t, setup.account, result, direction.model, direction.finalTier)
					})
				}
			}
		}
	}
}

func TestUltrafastForwardCompactFallbackHonorsFinalModelBlock(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, tc := range []struct{ model, fallback, blockedTier string }{
			{"gpt-6.2-sol", "gpt-6.1-sol", "priority"},
			{"gpt-6.1-sol", "gpt-6.2-sol", "ultrafast"},
		} {
			t.Run(fmt.Sprintf("passthrough=%t/%s", passthrough, tc.blockedTier), func(t *testing.T) {
				setup := newAstraOAuthSetup(t, passthrough)
				setup.account.Credentials["plan_type"] = "pro"
				setup.svc.cfg.Gateway.OpenAICompactModel = tc.fallback
				MarkOpenAINativeCompactionV2(setup.c)
				setup.svc.settingService = newOpenAIGatewayServiceWithSettings(t, &OpenAIFastPolicySettings{Rules: []OpenAIFastPolicyRule{{
					ServiceTier: tc.blockedTier, Scope: BetaPolicyScopeAll, Action: BetaPolicyActionPass,
					ModelWhitelist: []string{tc.model}, FallbackAction: BetaPolicyActionBlock, FallbackErrorMessage: "fallback tier disabled",
				}}}).settingService
				setup.upstream.resp = &http.Response{
					StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": []string{"application/json"}},
					Body: io.NopCloser(strings.NewReader(`{"error":{"code":"context_length_exceeded","message":"context window exceeded"}}`)),
				}
				body := []byte(fmt.Sprintf(`{"model":%q,"service_tier":"ultrafast","stream":false,"input":[{"type":"message","role":"user","content":"hello"},{"type":"compaction_trigger"}]}`, tc.model))
				result, err := setup.svc.Forward(context.Background(), setup.c, setup.account, body)
				var blocked *OpenAIFastBlockedError
				require.ErrorAs(t, err, &blocked)
				require.Equal(t, "fallback tier disabled", blocked.Message)
				require.Nil(t, result)
				require.Len(t, setup.upstream.bodies, 1, "blocked fallback must not reach upstream")
			})
		}
	}
}

func TestUltrafastForwardWebSocketRechecksEachTurnModel(t *testing.T) {
	for _, mode := range []string{OpenAIWSIngressModePassthrough, OpenAIWSIngressModeCtxPool} {
		t.Run(mode, func(t *testing.T) {
			testUltrafastForwardWebSocketTurns(t, mode)
		})
	}
}

func testUltrafastForwardWebSocketTurns(t *testing.T, mode string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg := passthroughLifecycleConfig()
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 5
	cfg.Gateway.OpenAIWS.IngressInterTurnIdleTimeoutSeconds = 5
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	upstream := newStagedPassthroughConn()
	defer upstream.Close()
	svc := newPassthroughLifecycleService(cfg, upstream)
	if mode == OpenAIWSIngressModeCtxPool {
		svc.openaiWSPool = newOpenAIWSConnPool(cfg)
		svc.openaiWSPool.setClientDialerForTest(&openAIWSSingleConnDialer{conn: &ultrafastStagedWSConn{upstream}})
	}
	account := &Account{
		ID: 902, Name: "ultrafast-turn-entitlement", Platform: PlatformOpenAI,
		Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "chatgpt-acc", "plan_type": "pro"},
		Extra:       map[string]any{"openai_oauth_responses_websockets_v2_mode": mode},
	}
	results := make(chan *OpenAIForwardResult, 2)
	server, serverErr := startPassthroughLifecycleServerWithHooks(t, ctx, svc, account, func(*gin.Context) *OpenAIWSIngressHooks {
		return &OpenAIWSIngressHooks{AfterTurn: func(_ int, result *OpenAIForwardResult, err error) {
			if err == nil && result != nil {
				results <- result
			}
		}}
	})
	defer server.Close()
	client := dialPassthroughLifecycleClientWithPayload(t, server, `{"type":"response.create","model":"gpt-6.2-sol","service_tier":"ultrafast","input":"hello"}`)
	defer client.CloseNow()

	for i, tc := range []struct{ model, tier string }{{"gpt-6.2-sol", "ultrafast"}, {"gpt-6.1-sol", "priority"}} {
		if i > 0 {
			err := client.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-6.1-sol","service_tier":"ultrafast","input":"hello"}`))
			require.NoError(t, err)
		}
		request := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
		require.Equal(t, tc.model, gjson.GetBytes(request, "model").String())
		require.Equal(t, tc.tier, gjson.GetBytes(request, "service_tier").String())
		upstream.Send(fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_ultrafast_turn_%d","model":%q,"status":"completed","service_tier":"default","usage":{"input_tokens":100,"output_tokens":50}}}`, i+1, tc.model))
		event, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
		require.NoError(t, err)
		require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())
		select {
		case result := <-results:
			require.NotNil(t, result.ServiceTier)
			require.Equal(t, tc.tier, *result.ServiceTier)
			require.Equal(t, tc.model, forwardResultBillingModel(result.Model, result.UpstreamModel))
			assertUltrafastForwardBilling(t, account, result, tc.model, tc.tier)
		case <-ctx.Done():
			t.Fatal("websocket turn did not produce billing metadata")
		}
	}
	_ = client.Close(coderws.StatusNormalClosure, "done")
	select {
	case <-serverErr:
	case <-ctx.Done():
		t.Fatal("websocket session did not stop")
	}
}

type ultrafastStagedWSConn struct{ *stagedPassthroughConn }

func (c *ultrafastStagedWSConn) WriteJSON(ctx context.Context, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.WriteFrame(ctx, coderws.MessageText, payload)
}

func assertUltrafastForwardBilling(t *testing.T, account *Account, result *OpenAIForwardResult, model, tier string) {
	t.Helper()
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billing := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	require.NoError(t, billing.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: result, APIKey: &APIKey{ID: 21}, User: &User{ID: 22}, Account: account,
	}))
	require.NotNil(t, usageRepo.lastLog)
	require.NotNil(t, usageRepo.lastLog.ServiceTier)
	require.Equal(t, tier, *usageRepo.lastLog.ServiceTier)
	tokens := UsageTokens{InputTokens: 100, OutputTokens: 50}
	wantCost, err := billing.billingService.CalculateCostWithServiceTier(model, tokens, 1, tier)
	require.NoError(t, err)
	require.InDelta(t, wantCost.TotalCost, usageRepo.lastLog.TotalCost, 1e-10)
	if tier == "priority" {
		standard, err := billing.billingService.CalculateCost(model, tokens, 1)
		require.NoError(t, err)
		require.InDelta(t, standard.TotalCost*2, usageRepo.lastLog.TotalCost, 1e-10, "downgraded requests must charge Fast, not Ultra")
	}
}
