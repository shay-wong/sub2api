package service

import (
	"context"
	"encoding/json"
	"strings"
)

const (
	featureKeyCodexImageGenerationBridge              = "codex_image_generation_bridge"
	featureKeyCodexImageGenerationPolicyAllowNonCodex = "codex_image_generation_policy_allow_non_codex"
)

const (
	featureKeyCodexImageGenerationExplicitToolPolicy = "codex_image_generation_explicit_tool_policy"

	codexImageGenerationExplicitToolPolicyAllow = "allow"
	codexImageGenerationExplicitToolPolicyStrip = "strip"
)

func boolOverridePtr(v bool) *bool {
	return &v
}

func boolOverrideFromMap(values map[string]any, keys ...string) *bool {
	if values == nil {
		return nil
	}
	for _, key := range keys {
		if v, ok := values[key].(bool); ok {
			return boolOverridePtr(v)
		}
	}
	return nil
}

func stringOverrideFromMap(values map[string]any, keys ...string) (string, bool) {
	if values == nil {
		return "", false
	}
	for _, key := range keys {
		if v, ok := values[key].(string); ok {
			return v, true
		}
	}
	return "", false
}

func normalizeCodexImageGenerationExplicitToolPolicy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case codexImageGenerationExplicitToolPolicyStrip, "remove", "drop":
		return codexImageGenerationExplicitToolPolicyStrip
	default:
		return codexImageGenerationExplicitToolPolicyAllow
	}
}

func platformBoolOverride(values map[string]any, key string, platform string) *bool {
	if values == nil {
		return nil
	}
	if v, ok := values[key].(bool); ok {
		return boolOverridePtr(v)
	}
	raw, ok := values[key].(map[string]any)
	if !ok {
		return nil
	}
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return nil
	}
	if v, ok := raw[platform].(bool); ok {
		return boolOverridePtr(v)
	}
	return nil
}

// CodexImageGenerationBridgeOverride returns the channel-level override for Codex
// image_generation bridge injection. Nil means follow the global/account policy.
func (c *Channel) CodexImageGenerationBridgeOverride(platform string) *bool {
	if c == nil {
		return nil
	}
	return platformBoolOverride(c.FeaturesConfig, featureKeyCodexImageGenerationBridge, platform)
}

// CodexImageGenerationBridgeOverride returns the account-level override for Codex
// image_generation bridge injection. Nil means follow the channel/global policy.
func (a *Account) CodexImageGenerationBridgeOverride() *bool {
	if a == nil || a.Platform != PlatformOpenAI || a.Extra == nil {
		return nil
	}
	if override := boolOverrideFromMap(a.Extra, featureKeyCodexImageGenerationBridge, "codex_image_generation_bridge_enabled"); override != nil {
		return override
	}
	openaiConfig, _ := a.Extra[PlatformOpenAI].(map[string]any)
	return boolOverrideFromMap(openaiConfig, featureKeyCodexImageGenerationBridge, "codex_image_generation_bridge_enabled")
}

// CodexImageGenerationPolicyAllowsNonCodex reports whether non-Codex clients
// participate in the account's Codex image-generation policy.
func (a *Account) CodexImageGenerationPolicyAllowsNonCodex() bool {
	if a == nil || a.Platform != PlatformOpenAI || a.Extra == nil {
		return false
	}
	if override := boolOverrideFromMap(a.Extra, featureKeyCodexImageGenerationPolicyAllowNonCodex); override != nil {
		return *override
	}
	openaiConfig, _ := a.Extra[PlatformOpenAI].(map[string]any)
	override := boolOverrideFromMap(openaiConfig, featureKeyCodexImageGenerationPolicyAllowNonCodex)
	return override != nil && *override
}

func allowsCodexImageGenerationBridgeForRequest(account *Account, isCodexCLI bool, body []byte) bool {
	return isCodexCLI || (account.CodexImageGenerationPolicyAllowsNonCodex() && openAIRequestBodyHasNativeImageGenerationDeclaration(body))
}

// CodexImageGenerationExplicitToolPolicy returns the account-level policy for
// client-provided Codex /responses image_generation tools. Unknown or unset
// values default to allow to preserve existing behavior.
func (a *Account) CodexImageGenerationExplicitToolPolicy() string {
	if a == nil || a.Platform != PlatformOpenAI || a.Extra == nil {
		return codexImageGenerationExplicitToolPolicyAllow
	}
	if policy, ok := stringOverrideFromMap(a.Extra, featureKeyCodexImageGenerationExplicitToolPolicy); ok {
		return normalizeCodexImageGenerationExplicitToolPolicy(policy)
	}
	openaiConfig, _ := a.Extra[PlatformOpenAI].(map[string]any)
	if policy, ok := stringOverrideFromMap(openaiConfig, featureKeyCodexImageGenerationExplicitToolPolicy); ok {
		return normalizeCodexImageGenerationExplicitToolPolicy(policy)
	}
	return codexImageGenerationExplicitToolPolicyAllow
}

// applyOpenAIWSImageGenerationPolicy keeps account image policy behavior aligned
// across pooled and passthrough Responses WebSocket transports.
func (s *OpenAIGatewayService) applyOpenAIWSImageGenerationPolicy(
	ctx context.Context,
	account *Account,
	apiKey *APIKey,
	isCodexCLI bool,
	payload []byte,
) ([]byte, error) {
	policyClientAllowed := isCodexCLI || account.CodexImageGenerationPolicyAllowsNonCodex()
	toolPolicy := codexImageGenerationExplicitToolPolicyAllow
	if policyClientAllowed {
		toolPolicy = account.CodexImageGenerationExplicitToolPolicy()
	}
	if toolPolicy == codexImageGenerationExplicitToolPolicyStrip {
		stripped, changed, err := stripOpenAIImageGenerationToolsFromRawPayload(payload)
		if err != nil {
			return payload, err
		}
		if changed {
			logOpenAIWSModeInfo("ingress_ws_image_tool_stripped_by_policy account_id=%d", account.ID)
		}
		return stripped, nil
	}

	bridgeEnabled := allowsCodexImageGenerationBridgeForRequest(account, isCodexCLI, payload) &&
		!isOpenAIResponsesLiteWebSocketPayload(payload) &&
		GroupAllowsImageGeneration(apiKeyGroup(apiKey)) &&
		s.isCodexImageGenerationBridgeEnabled(ctx, account, apiKey)
	if !bridgeEnabled {
		return payload, nil
	}

	payloadMap := make(map[string]any)
	if err := json.Unmarshal(payload, &payloadMap); err != nil {
		return payload, err
	}
	modified := false
	if ensureOpenAIResponsesImageGenerationTool(payloadMap) {
		modified = true
		logOpenAIWSModeInfo("ingress_ws_codex_image_tool_injected account_id=%d", account.ID)
	}
	if ensureOpenAIResponsesImageGenerationToolChoiceAuto(payloadMap) {
		modified = true
		logOpenAIWSModeInfo("ingress_ws_codex_image_tool_choice_auto account_id=%d", account.ID)
	}
	if normalizeOpenAIResponsesImageGenerationTools(payloadMap) {
		modified = true
	}
	if applyCodexImageGenerationBridgeInstructions(payloadMap) {
		modified = true
		logOpenAIWSModeInfo("ingress_ws_codex_image_bridge_instructions_added account_id=%d", account.ID)
	}
	if !modified {
		return payload, nil
	}
	rebuilt, err := json.Marshal(payloadMap)
	if err != nil {
		return payload, err
	}
	return rebuilt, nil
}
