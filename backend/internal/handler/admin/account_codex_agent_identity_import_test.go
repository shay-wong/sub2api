package admin

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestNormalizeCodexImportEntryAcceptsAgentIdentityAuthJSON(t *testing.T) {
	value := newCodexAgentIdentityImportValue(t, "account-import", "user-import")
	item, err := normalizeCodexImportEntry(codexImportEntry{
		Index: 1,
		Value: value,
	})
	require.NoError(t, err)
	require.NotNil(t, item)
	require.True(t, item.IsAgentIdentity)
	agentIdentity, ok := value["agent_identity"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, service.OpenAIAuthModeAgentIdentity, item.Credentials["auth_mode"])
	require.Equal(t, "runtime-import", item.Credentials["agent_runtime_id"])
	require.Equal(t, agentIdentity["agent_private_key"], item.Credentials["agent_private_key"])
	require.Equal(t, "account-import", item.Credentials["chatgpt_account_id"])
	require.Equal(t, "user-import", item.Credentials["chatgpt_user_id"])
	require.NotContains(t, item.Credentials, "access_token")
	require.NotContains(t, item.Credentials, "refresh_token")
	require.NotEmpty(t, item.WarningTexts)
}

func TestImportCodexAgentIdentityWithoutExpiryCreatesAccount(t *testing.T) {
	svc := newCodexImportMemoryAdminService(nil)
	handler := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	result, err := handler.importCodexSessions(
		context.Background(),
		CodexSessionImportRequest{SkipDefaultGroupBind: boolPtr(true)},
		[]codexImportEntry{{Index: 1, Value: newCodexAgentIdentityImportValue(t, "account-create", "user-create")}},
	)

	require.NoError(t, err)
	require.Equal(t, 1, result.Created)
	require.Zero(t, result.Failed)
	require.Len(t, svc.createdAccounts, 1)
	require.Equal(t, service.OpenAIAuthModeAgentIdentity, svc.createdAccounts[0].Credentials["auth_mode"])
	require.NotContains(t, svc.createdAccounts[0].Credentials, "access_token")
	require.NotContains(t, svc.createdAccounts[0].Credentials, "refresh_token")
	require.Nil(t, svc.createdAccounts[0].ExpiresAt)
}

func TestImportCodexAgentIdentityReplacesExistingOAuthCredentials(t *testing.T) {
	existing := service.Account{
		ID:       71,
		Name:     "existing-oauth",
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Credentials: map[string]any{
			"chatgpt_account_id": "account-update",
			"chatgpt_user_id":    "user-update",
			"access_token":       "stale-access",
			"refresh_token":      "stale-refresh",
			"id_token":           "stale-id",
			"expires_at":         "2026-08-05T13:40:42Z",
			"client_id":          "stale-client",
			"openai_auth_mode":   "oauth",
			"token_type":         "Bearer",
			"scope":              "openid profile",
			"model_mapping":      map[string]any{"gpt-old": "gpt-new"},
		},
	}
	svc := newCodexImportMemoryAdminService([]service.Account{existing})
	handler := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	result, err := handler.importCodexSessions(
		context.Background(),
		CodexSessionImportRequest{SkipDefaultGroupBind: boolPtr(true)},
		[]codexImportEntry{{Index: 1, Value: newCodexAgentIdentityImportValue(t, "account-update", "user-update")}},
	)

	require.NoError(t, err)
	require.Equal(t, 1, result.Updated)
	require.Zero(t, result.Failed)
	require.Len(t, svc.updatedAccounts, 1)
	credentials := svc.updatedAccounts[0].input.Credentials
	require.True(t, svc.updatedAccounts[0].input.ReplaceCredentials)
	require.Equal(t, service.OpenAIAuthModeAgentIdentity, credentials["auth_mode"])
	for _, key := range []string{"access_token", "refresh_token", "id_token", "expires_at", "client_id", "openai_auth_mode", "token_type", "scope"} {
		require.NotContains(t, credentials, key)
	}
	require.Contains(t, credentials, "model_mapping")
	for _, warning := range result.Warnings {
		require.NotContains(t, warning.Message, "保留自动续期凭据")
	}
}

func newCodexAgentIdentityImportValue(t *testing.T, accountID, userID string) map[string]any {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	privateKeyBase64 := base64.StdEncoding.EncodeToString(der)
	return map[string]any{
		"auth_mode": "agentIdentity",
		"agent_identity": map[string]any{
			"agent_runtime_id":           "runtime-import",
			"agent_private_key":          privateKeyBase64,
			"account_id":                 strings.TrimSpace(accountID),
			"chatgpt_user_id":            strings.TrimSpace(userID),
			"email":                      "agent@example.invalid",
			"plan_type":                  "pro",
			"chatgpt_account_is_fedramp": false,
		},
	}
}
