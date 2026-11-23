package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIUltrafastFallback(t *testing.T) {
	for _, tc := range []struct {
		name, kind, plan, model, want string
	}{
		{"Pro 200 legacy Sol", AccountTypeOAuth, "pro", "gpt-6.1-sol", "priority"},
		{"Pro 200 Astra", AccountTypeOAuth, "pro", "gpt-6-astra", "priority"},
		{"Pro 200 new Sol", AccountTypeOAuth, "pro", "gpt-6.2-sol", "ultrafast"},
		{"Pro 200 new Astra", AccountTypeOAuth, "pro", "gpt-6.2-astra", "priority"},
		{"Pro 500 legacy Sol", AccountTypeOAuth, "promax", "gpt-6.1-sol", "ultrafast"},
		{"Pro 500 unsupported model", AccountTypeOAuth, "promax", "gpt-5.6-sol", "priority"},
		{"unknown subscription", AccountTypeOAuth, "", "gpt-6.1-sol", "priority"},
		{"setup token", AccountTypeSetupToken, "pro", "gpt-6.1-sol", "priority"},
		{"API key", AccountTypeAPIKey, "", "gpt-6.1-sol", "ultrafast"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := &Account{Platform: PlatformOpenAI, Type: tc.kind, Credentials: map[string]any{"plan_type": tc.plan}}
			for _, groupForced := range []bool{false, true} {
				ctx := context.Background()
				body := []byte(`{"model":"public","service_tier":"ultrafast","input":"hello"}`)
				if groupForced {
					ctx = context.WithValue(ctx, ctxkey.Group, &Group{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, ForceOpenAIUltrafast: true})
					body = []byte(`{"model":"public","input":"hello"}`)
				}
				svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
				out, err := svc.applyOpenAIFastPolicyToBody(ctx, account, tc.model, body)
				require.NoError(t, err)
				require.Equal(t, tc.want, gjson.GetBytes(out, "service_tier").String(), "group forced=%t", groupForced)
				require.Equal(t, "hello", gjson.GetBytes(out, "input").String())
				frame := append([]byte(`{"type":"response.create",`), body[1:]...)
				out, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(ctx, account, tc.model, frame)
				require.NoError(t, err)
				require.Nil(t, blocked)
				require.Equal(t, tc.want, gjson.GetBytes(out, "service_tier").String(), "WebSocket group forced=%t", groupForced)
			}
		})
	}
}

func TestOpenAIUltrafastFallbackRespectsPolicies(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"plan_type": "pro"}}
	body := []byte(`{"model":"gpt-6.1-sol","service_tier":"ultrafast"}`)
	for _, tier := range []string{OpenAIFastTierUltrafast, OpenAIFastTierPriority} {
		for _, action := range []string{BetaPolicyActionFilter, BetaPolicyActionBlock} {
			t.Run(tier+"/"+action, func(t *testing.T) {
				svc := newOpenAIGatewayServiceWithSettings(t, &OpenAIFastPolicySettings{Rules: []OpenAIFastPolicyRule{{
					ServiceTier: tier, Scope: BetaPolicyScopeAll, Action: action, ErrorMessage: "denied",
				}}})
				out, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-6.1-sol", body)
				if action == BetaPolicyActionBlock {
					var blocked *OpenAIFastBlockedError
					require.ErrorAs(t, err, &blocked)
					require.Equal(t, "denied", blocked.Message)
				} else {
					require.NoError(t, err)
					require.False(t, gjson.GetBytes(out, "service_tier").Exists())
				}
				out, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, "gpt-6.1-sol", append([]byte(`{"type":"response.create",`), body[1:]...))
				require.NoError(t, err)
				if action == BetaPolicyActionBlock {
					require.NotNil(t, blocked)
					require.Equal(t, "denied", blocked.Message)
				} else {
					require.Nil(t, blocked)
					require.False(t, gjson.GetBytes(out, "service_tier").Exists())
				}
			})
		}
	}
}

func TestOpenAIUltrafastFallbackUsesShadowCredential(t *testing.T) {
	parentID := int64(1)
	shadow := &Account{ID: 2, ParentAccountID: &parentID, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{
		"plan_type": "promax", "model_mapping": map[string]any{"public": "gpt-6.1-sol"},
	}}
	parent := &Account{ID: parentID, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{
		"plan_type": "pro", "model_mapping": map[string]any{"public": "gpt-6.2-sol"},
	}}
	repo := &codexAccountIdentityRepoStub{account: parent}
	svc := &OpenAIGatewayService{accountRepo: repo}
	finalModel := resolveOpenAIAccountUpstreamModelForRequest(shadow, "public", false)
	require.Equal(t, "gpt-6.1-sol", finalModel)
	body := []byte(`{"service_tier":"ultrafast"}`)
	out, err := svc.applyOpenAIFastPolicyToBody(context.Background(), shadow, finalModel, body)
	require.NoError(t, err)
	require.Equal(t, OpenAIFastTierPriority, gjson.GetBytes(out, "service_tier").String())
	parent.Credentials["plan_type"] = "promax"
	out, err = svc.applyOpenAIFastPolicyToBody(context.Background(), shadow, finalModel, body)
	require.NoError(t, err)
	require.Equal(t, OpenAIFastTierUltrafast, gjson.GetBytes(out, "service_tier").String())
	repo.account = nil
	_, err = svc.applyOpenAIFastPolicyToBody(context.Background(), shadow, finalModel, body)
	require.ErrorContains(t, err, "not found")
}

func TestOpenAIUltrafastFallbackGrantIsModelSpecific(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"plan_type": "enterprise"}}
	account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
		"gpt-6.2-sol": {ID: "gpt-6.2-sol", CodexToolCapabilities: map[string]json.RawMessage{"service_tiers": json.RawMessage(`[{"id":"ultrafast"}]`)}},
	}})
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	for _, model := range []string{"gpt-6.1-sol", "gpt-6.2-sol"} {
		out, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, model, []byte(`{"service_tier":"ultrafast"}`))
		require.NoError(t, err)
		want := "priority"
		if model == "gpt-6.2-sol" {
			want = "ultrafast"
		}
		require.Equal(t, want, gjson.GetBytes(out, "service_tier").String(), model)
	}
}
