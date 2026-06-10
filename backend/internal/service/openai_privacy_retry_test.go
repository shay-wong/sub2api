//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

func TestAdminService_EnsureOpenAIPrivacy_RetriesNonSuccessModes(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{PrivacyModeFailed, PrivacyModeCFBlocked, PrivacyModeHTMLBlocked} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			privacyCalls := 0
			svc := &adminServiceImpl{
				accountRepo: &mockAccountRepoForGemini{},
				privacyClientFactory: func(proxyURL string) (*req.Client, error) {
					privacyCalls++
					return nil, errors.New("factory failed")
				},
			}

			account := &Account{
				ID:       101,
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Credentials: map[string]any{
					"access_token": "token-1",
				},
				Extra: map[string]any{
					"privacy_mode": mode,
				},
			}

			got := svc.EnsureOpenAIPrivacy(context.Background(), account)

			require.Equal(t, PrivacyModeFailed, got)
			require.Equal(t, 1, privacyCalls)
		})
	}
}

func TestTokenRefreshService_ensureOpenAIPrivacy_RetriesNonSuccessModes(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          1,
			RetryBackoffSeconds: 0,
		},
	}

	for _, mode := range []string{PrivacyModeFailed, PrivacyModeCFBlocked, PrivacyModeHTMLBlocked} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			service := NewTokenRefreshService(&tokenRefreshAccountRepo{}, nil, nil, nil, nil, nil, nil, cfg, nil)
			privacyCalls := 0
			service.SetPrivacyDeps(func(proxyURL string) (*req.Client, error) {
				privacyCalls++
				return nil, errors.New("factory failed")
			}, nil)

			account := &Account{
				ID:       202,
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Credentials: map[string]any{
					"access_token": "token-2",
				},
				Extra: map[string]any{
					"privacy_mode": mode,
				},
			}

			service.ensureOpenAIPrivacy(context.Background(), account)

			require.Equal(t, 1, privacyCalls)
		})
	}
}

func TestClassifyOpenAIPrivacyFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		headers    http.Header
		body       string
		want       string
	}{
		{
			name:       "cloudflare marker wins over html",
			statusCode: 403,
			body:       `<html><title>Just a moment...</title><script src="/cdn-cgi/challenge-platform/h/b/orchestrate"></script></html>`,
			want:       PrivacyModeCFBlocked,
		},
		{
			name:       "cloudflare challenge header wins without body marker",
			statusCode: 403,
			headers: http.Header{
				"Cf-Mitigated": []string{"challenge"},
			},
			body: `<html><body>Forbidden</body></html>`,
			want: PrivacyModeCFBlocked,
		},
		{
			name:       "cloudflare ray header alone keeps html classification",
			statusCode: 403,
			headers: http.Header{
				"Content-Type": []string{"text/html; charset=utf-8"},
				"Cf-Ray":       []string{"abc-SJC"},
			},
			body: `<html><body>Access denied</body></html>`,
			want: PrivacyModeHTMLBlocked,
		},
		{
			name:       "cloudflare server header alone keeps html classification",
			statusCode: 503,
			headers: http.Header{
				"Content-Type": []string{"text/html"},
				"Server":       []string{"cloudflare"},
			},
			body: `<html><body>Service unavailable</body></html>`,
			want: PrivacyModeHTMLBlocked,
		},
		{
			name:       "html content type without cf marker",
			statusCode: 403,
			headers: http.Header{
				"Content-Type": []string{"text/html; charset=utf-8"},
			},
			body: `<html><head><title>Forbidden</title></head><body>Access denied</body></html>`,
			want: PrivacyModeHTMLBlocked,
		},
		{
			name:       "html prefix without content type",
			statusCode: 403,
			body:       `<!doctype html><html><body>Forbidden</body></html>`,
			want:       PrivacyModeHTMLBlocked,
		},
		{
			name:       "json 403 remains generic failure",
			statusCode: 403,
			body:       `{"error":"forbidden"}`,
			want:       PrivacyModeFailed,
		},
		{
			name:       "non blocking status remains generic failure",
			statusCode: 500,
			body:       `<html><body>error</body></html>`,
			want:       PrivacyModeFailed,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, classifyOpenAIPrivacyFailure(tt.statusCode, tt.headers, tt.body))
		})
	}
}
