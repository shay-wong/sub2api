//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func newSessionBindingAuthService(t *testing.T) (*AuthService, *refreshTokenCacheStub, *User) {
	t.Helper()
	user := &User{
		ID:           42,
		Email:        "session@example.com",
		Role:         RoleUser,
		Status:       StatusActive,
		TokenVersion: 1,
	}
	cfg := &config.Config{JWT: config.JWTConfig{
		Secret:                   "test-session-binding-secret",
		AccessTokenExpireMinutes: 60,
		RefreshTokenExpireDays:   7,
	}}
	settings := NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeySessionBindingEnabled: "true",
	}}, cfg)
	cache := &refreshTokenCacheStub{}
	svc := NewAuthService(
		nil,
		&userRepoStub{user: user},
		nil,
		cache,
		cfg,
		settings,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	return svc, cache, user
}

func sessionBindingTestContext(clientIP, peerIP string) context.Context {
	return WithSessionBinding(context.Background(), &SessionBinding{
		IP:        clientIP,
		IPSource:  SessionBindingIPSourceTrustedForwarded,
		PeerIP:    peerIP,
		UserAgent: "test-agent",
	})
}

func TestTokenPairStoresSeparatedSessionBindingFingerprint(t *testing.T) {
	svc, cache, user := newSessionBindingAuthService(t)
	pair, err := svc.GenerateTokenPair(sessionBindingTestContext("203.0.113.10", "172.24.149.226"), user, "")
	require.NoError(t, err)

	claims, err := svc.ValidateToken(pair.AccessToken)
	require.NoError(t, err)
	require.NotEmpty(t, claims.BindingClientIPHash)
	require.NotEmpty(t, claims.BindingUserAgentHash)
	require.Equal(t, SessionBindingIPSourceTrustedForwarded, claims.BindingIPSource)

	stored, err := cache.GetRefreshToken(context.Background(), hashToken(pair.RefreshToken))
	require.NoError(t, err)
	require.Equal(t, claims.BindingClientIPHash, stored.BindingClientIPHash)
	require.Equal(t, claims.BindingUserAgentHash, stored.BindingUserAgentHash)
	require.Equal(t, claims.BindingIPSource, stored.BindingIPSource)
}

func TestRefreshTokenPairIgnoresProxyPeerChange(t *testing.T) {
	svc, _, user := newSessionBindingAuthService(t)
	pair, err := svc.GenerateTokenPair(sessionBindingTestContext("203.0.113.10", "172.24.149.226"), user, "")
	require.NoError(t, err)

	refreshed, err := svc.RefreshTokenPair(
		sessionBindingTestContext("203.0.113.10", "172.24.149.227"),
		pair.RefreshToken,
	)
	require.NoError(t, err)
	require.NotEmpty(t, refreshed.AccessToken)
}

func TestRefreshTokenPairRejectsClientIPChange(t *testing.T) {
	svc, cache, user := newSessionBindingAuthService(t)
	pair, err := svc.GenerateTokenPair(sessionBindingTestContext("203.0.113.10", "172.24.149.226"), user, "")
	require.NoError(t, err)

	refreshCtx := sessionBindingTestContext("203.0.113.11", "172.24.149.226")
	_, err = svc.RefreshTokenPair(
		refreshCtx,
		pair.RefreshToken,
	)
	require.ErrorIs(t, err, ErrSessionBindingMismatch)
	require.NotEmpty(t, cache.deletedFamilies)
	require.Equal(t, SessionBindingClientIPMismatch, SessionBindingFromContext(refreshCtx).MismatchReason())
}

func TestRefreshTokenPairMigratesUnchangedLegacyFingerprint(t *testing.T) {
	svc, cache, user := newSessionBindingAuthService(t)
	pair, err := svc.GenerateTokenPair(sessionBindingTestContext("172.24.149.226", "172.24.149.226"), user, "")
	require.NoError(t, err)

	tokenHash := hashToken(pair.RefreshToken)
	legacyData := cache.tokens[tokenHash]
	require.NotNil(t, legacyData)
	legacyData.BindingClientIPHash = ""
	legacyData.BindingIPSource = ""
	legacyData.BindingUserAgentHash = ""

	refreshed, err := svc.RefreshTokenPair(
		sessionBindingTestContext("172.24.149.226", "172.24.149.227"),
		pair.RefreshToken,
	)
	require.NoError(t, err)
	require.Empty(t, cache.deletedFamilies)

	claims, err := svc.ValidateToken(refreshed.AccessToken)
	require.NoError(t, err)
	require.NotEmpty(t, claims.BindingClientIPHash)
	require.NotEmpty(t, claims.BindingUserAgentHash)
	require.Equal(t, SessionBindingIPSourceTrustedForwarded, claims.BindingIPSource)
}
