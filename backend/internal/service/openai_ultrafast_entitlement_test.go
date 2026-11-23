//go:build unit

package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIAccountSupportsUltrafast(t *testing.T) {
	const ultra = `[{"id":"priority"},{"id":"ultrafast"}]`
	for _, tc := range []struct {
		name, accountType, plan, model, metadataModel, tiers string
		want                                                 bool
	}{
		{name: "pro 200 sol 61", plan: "pro", model: "gpt-6.1-sol"},
		{name: "pro 200 astra 6", plan: "pro", model: "gpt-6-astra"},
		{name: "pro 200 astra 62", plan: "pro", model: "gpt-6.2-astra"},
		{name: "pro 200 sol 62", plan: "pro", model: "gpt-6.2-sol", want: true},
		{name: "legacy pro spelling", plan: " ChatGPT_Pro ", model: "gpt-6.2-sol-max", want: true},
		{name: "pro 200 older sol fast-only snapshot", plan: "pro", model: "gpt-6.2-sol", tiers: `[{"id":"priority"}]`, want: true},
		{name: "pro 200 sol 61 stale ultra grant", plan: "pro", model: "gpt-6.1-sol", tiers: ultra},
		{name: "pro 500 astra 6", plan: "promax", model: "gpt-6-astra", want: true},
		{name: "pro 500 sol 61", plan: "promax", model: "gpt-6.1-sol-max", want: true},
		{name: "pro 500 astra 62", plan: "Pro-Max", model: "gpt-6.2-astra", want: true},
		{name: "pro 500 sol 62", plan: "promax", model: "gpt-6.2-sol", want: true},
		{name: "pro 500 unsupported model", plan: "promax", model: "gpt-6.1-terra", tiers: ultra},
		{name: "pro 100 cannot inherit a grant", plan: "prolite", model: "gpt-6.2-sol", tiers: ultra},
		{name: "plus cannot inherit a grant", plan: "plus", model: "gpt-6.1-sol", tiers: ultra},
		{name: "business cannot inherit a grant", plan: "business", model: "gpt-6.1-sol", tiers: ultra},
		{name: "team cannot inherit a grant", plan: "team", model: "gpt-6.1-sol", tiers: ultra},
		{name: "enterprise same model grant", plan: "enterprise", model: "gpt-6.1-sol", tiers: ultra, want: true},
		{name: "enterprise effort alias grant", plan: "enterprise", model: "gpt-6.1-sol-max", metadataModel: "gpt-6.1-sol", tiers: ultra, want: true},
		{name: "enterprise other model grant", plan: "enterprise", model: "gpt-6.1-sol", metadataModel: "gpt-6.2-sol", tiers: ultra},
		{name: "enterprise not discovered", plan: "enterprise", model: "gpt-6.1-sol"},
		{name: "edu same model grant", plan: "edu", model: "gpt-6.2-astra", tiers: ultra, want: true},
		{name: "edu fast only", plan: "edu", model: "gpt-6.2-sol", tiers: `[{"id":"priority"}]`},
		{name: "unknown no grant", plan: "unknown", model: "gpt-6.2-sol"},
		{name: "unknown same model grant", plan: "future-enterprise", model: "gpt-6.2-sol", tiers: ultra, want: true},
		{name: "missing plan no grant", model: "gpt-6.1-sol"},
		{name: "missing plan empty tiers", model: "gpt-6.1-sol", tiers: `[]`},
		{name: "missing plan null tiers", model: "gpt-6.1-sol", tiers: `null`},
		{name: "unknown malformed grant", plan: "unknown", model: "gpt-6.1-sol", tiers: `{"id":"ultrafast"}`},
		{name: "unknown scalar grant", plan: "unknown", model: "gpt-6.1-sol", tiers: `"ultrafast"`},
		{name: "setup token same rules", accountType: AccountTypeSetupToken, plan: "pro", model: "gpt-6.1-sol"},
		{name: "api key ignores subscription plan", accountType: AccountTypeAPIKey, plan: "plus", model: "gpt-6.1-sol", want: true},
		{name: "api key unknown model", accountType: AccountTypeAPIKey, model: "future-model", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kind := tc.accountType
			if kind == "" {
				kind = AccountTypeOAuth
			}
			account := &Account{Platform: PlatformOpenAI, Type: kind, Credentials: map[string]any{"plan_type": tc.plan}}
			if tc.tiers != "" {
				model := tc.metadataModel
				if model == "" {
					model = tc.model
				}
				account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
					model: {ID: model, CodexToolCapabilities: map[string]json.RawMessage{"service_tiers": json.RawMessage(tc.tiers)}},
				}})
			}
			require.Equal(t, tc.want, openAIAccountSupportsUltrafast(account, tc.model))
		})
	}
	require.False(t, openAIAccountSupportsUltrafast(nil, "gpt-6.1-sol"))
	require.False(t, openAIAccountSupportsUltrafast(&Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}, "gpt-6.1-sol"))
}

func TestOAuthUltrafastCatalogMatchesAccountEntitlement(t *testing.T) {
	const declared = `[{"id":"priority","name":"Provider Fast","provider_field":7},{"id":"ultrafast","name":"Provider Ultra","provider_field":8}]`
	for _, tc := range []struct {
		plan, model, tiers string
		want               bool
	}{
		{"pro", "gpt-6.1-sol", declared, false},
		{"plus", "gpt-6.2-sol", declared, false},
		{"pro", "gpt-6.2-sol", `[{"id":"priority","name":"Provider Fast"}]`, true},
		{"promax", "gpt-6-astra", "", true},
		{"promax", "gpt-6.1-sol", "", true},
		{"enterprise", "gpt-6.1-sol", declared, true},
	} {
		t.Run(tc.plan+"/"+tc.model, func(t *testing.T) {
			account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"plan_type": tc.plan}}
			if tc.tiers != "" {
				account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
					tc.model: {ID: tc.model, CodexToolCapabilities: map[string]json.RawMessage{"service_tiers": json.RawMessage(tc.tiers)}},
				}})
			}
			caps := accountCodexToolCapabilities(account, tc.model)
			require.Equal(t, tc.want, gjson.GetBytes(caps["service_tiers"], `#(id=="ultrafast")`).Exists())
			if tc.tiers == declared {
				require.EqualValues(t, 7, gjson.GetBytes(caps["service_tiers"], `#(id=="priority").provider_field`).Int())
				if tc.want {
					require.EqualValues(t, 8, gjson.GetBytes(caps["service_tiers"], `#(id=="ultrafast").provider_field`).Int())
				}
			}
		})
	}
}

func TestOAuthUltrafastCatalogDoesNotInheritUnsupportedStaticTier(t *testing.T) {
	account := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"plan_type": "pro"}}
	body, err := buildCodexModelsManifestForAccounts(PlatformOpenAI, []string{"gpt-5.6-sol"}, []Account{account}, nil, nil, true)
	require.NoError(t, err)
	model := decodeCodexManifestModels(t, body)[0]
	tiers, err := json.Marshal(model["service_tiers"])
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(tiers, `#(id=="ultrafast")`).Exists())
	require.True(t, gjson.GetBytes(tiers, `#(id=="priority")`).Exists())
}
