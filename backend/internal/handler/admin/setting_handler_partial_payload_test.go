//go:build unit

package admin

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/stretchr/testify/require"
)

type acceptingTurnstileVerifier struct{}

func (acceptingTurnstileVerifier) VerifyToken(context.Context, string, string, string) (*service.TurnstileVerifyResponse, error) {
	return &service.TurnstileVerifyResponse{ErrorCodes: []string{"invalid-input-response"}}, nil
}

// Saving settings is a whole-document PUT. A client that sends only the field it
// cares about must not reset everything else: a payload as small as
// `{"risk_control_enabled":true}` used to clear site_name, after which
// getStringOrDefault rendered the empty value as the built-in default and the
// login page silently changed name.

func TestUpdateSettingsPartialPayloadKeepsUnsentKeys(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeySiteName:           "Example Gateway",
		service.SettingKeySiteSubtitle:       "Example Gateway Platform",
		service.SettingKeySMTPHost:           "smtp.example.com",
		service.SettingKeySMTPFrom:           "noreply@example.com",
		service.SettingKeyTurnstileEnabled:   "true",
		service.SettingKeyTurnstileSiteKey:   "site-key",
		service.SettingKeyTurnstileSecretKey: "secret-key",
	})

	rec := doUpdateSettings(t, h, map[string]any{"risk_control_enabled": true}, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, "true", repo.values[service.SettingKeyRiskControlEnabled],
		"the field the caller actually sent must be written")

	require.Equal(t, "Example Gateway", repo.values[service.SettingKeySiteName])
	require.Equal(t, "Example Gateway Platform", repo.values[service.SettingKeySiteSubtitle])
	require.Equal(t, "smtp.example.com", repo.values[service.SettingKeySMTPHost])
	require.Equal(t, "noreply@example.com", repo.values[service.SettingKeySMTPFrom])
	require.Equal(t, "true", repo.values[service.SettingKeyTurnstileEnabled])
}

// A full payload keeps whole-document semantics: fields explicitly set to their
// zero value are still cleared.
func TestUpdateSettingsFullPayloadStillClearsSentEmptyFields(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeySiteName: "Example Gateway",
	})

	rec := doUpdateSettings(t, h, map[string]any{"site_name": ""}, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, "", repo.values[service.SettingKeySiteName],
		"an explicitly sent empty value is a deliberate clear, not an omission")
}

// smtp_from_email is the one request field whose JSON name differs from its
// setting key; the alias keeps it from being treated as always-omitted.
func TestUpdateSettingsSMTPFromAliasIsWritable(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeySMTPFrom: "old@example.com",
	})

	rec := doUpdateSettings(t, h, map[string]any{"smtp_from_email": "new@example.com"}, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, "new@example.com", repo.values[service.SettingKeySMTPFrom])
}

// Partial updates must validate against the stored enabled state instead of
// treating omitted switches as false and allowing required values to be cleared.
func TestUpdateSettingsPartialPayloadValidatesAlreadyEnabledIntegrations(t *testing.T) {
	tests := []struct {
		name       string
		stored     map[string]string
		payload    map[string]any
		settingKey string
	}{
		{
			name: "turnstile",
			stored: map[string]string{
				service.SettingKeyTurnstileEnabled:   "true",
				service.SettingKeyTurnstileSiteKey:   "site-key",
				service.SettingKeyTurnstileSecretKey: "secret-key",
			},
			payload:    map[string]any{"turnstile_site_key": ""},
			settingKey: service.SettingKeyTurnstileSiteKey,
		},
		{
			name: "linuxdo oauth",
			stored: map[string]string{
				service.SettingKeyLinuxDoConnectEnabled:      "true",
				service.SettingKeyLinuxDoConnectClientID:     "client-id",
				service.SettingKeyLinuxDoConnectClientSecret: "client-secret",
				service.SettingKeyLinuxDoConnectRedirectURL:  "https://example.com/oauth/callback",
			},
			payload:    map[string]any{"linuxdo_connect_client_id": ""},
			settingKey: service.SettingKeyLinuxDoConnectClientID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, repo := newStepUpSwitchTestHandler(t, tt.stored)
			before := repo.values[tt.settingKey]

			rec := doUpdateSettings(t, h, tt.payload, nil)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Equal(t, before, repo.values[tt.settingKey])
		})
	}
}

func TestUpdateSettingsPartialPayloadUpdatesAlreadyEnabledTurnstile(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeyTurnstileEnabled:   "true",
		service.SettingKeyTurnstileSiteKey:   "old-site-key",
		service.SettingKeyTurnstileSecretKey: "secret-key",
	})
	h.turnstileService = service.NewTurnstileService(h.settingService, acceptingTurnstileVerifier{})

	rec := doUpdateSettings(t, h, map[string]any{"turnstile_site_key": "new-site-key"}, nil)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "new-site-key", repo.values[service.SettingKeyTurnstileSiteKey])
	require.Equal(t, "true", repo.values[service.SettingKeyTurnstileEnabled])
	require.Equal(t, "secret-key", repo.values[service.SettingKeyTurnstileSecretKey])
	_, wroteEnabled := repo.lastUpdates[service.SettingKeyTurnstileEnabled]
	_, wroteSecret := repo.lastUpdates[service.SettingKeyTurnstileSecretKey]
	require.False(t, wroteEnabled)
	require.False(t, wroteSecret)
}

func TestUpdateSettingsPartialPayloadUpdatesAlreadyEnabledOAuth(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeyLinuxDoConnectEnabled:      "true",
		service.SettingKeyLinuxDoConnectClientID:     "client-id",
		service.SettingKeyLinuxDoConnectClientSecret: "client-secret",
		service.SettingKeyLinuxDoConnectRedirectURL:  "https://example.com/old-callback",
	})

	rec := doUpdateSettings(t, h, map[string]any{
		"linuxdo_connect_redirect_url": "https://example.com/new-callback",
	}, nil)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "https://example.com/new-callback", repo.values[service.SettingKeyLinuxDoConnectRedirectURL])
	require.Equal(t, "true", repo.values[service.SettingKeyLinuxDoConnectEnabled])
	require.Equal(t, "client-id", repo.values[service.SettingKeyLinuxDoConnectClientID])
	require.Equal(t, "client-secret", repo.values[service.SettingKeyLinuxDoConnectClientSecret])
	_, wroteEnabled := repo.lastUpdates[service.SettingKeyLinuxDoConnectEnabled]
	_, wroteClientID := repo.lastUpdates[service.SettingKeyLinuxDoConnectClientID]
	_, wroteSecret := repo.lastUpdates[service.SettingKeyLinuxDoConnectClientSecret]
	require.False(t, wroteEnabled)
	require.False(t, wroteClientID)
	require.False(t, wroteSecret)
}
