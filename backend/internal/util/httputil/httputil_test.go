package httputil

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsCloudflareChallengeResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		headers    http.Header
		body       []byte
		want       bool
	}{
		{
			name:       "mitigated challenge header",
			statusCode: http.StatusForbidden,
			headers: http.Header{
				"Cf-Mitigated": []string{"challenge"},
			},
			body: []byte(`<html><body>Forbidden</body></html>`),
			want: true,
		},
		{
			name:       "service unavailable challenge marker",
			statusCode: http.StatusServiceUnavailable,
			headers: http.Header{
				"Content-Type": []string{"text/html; charset=utf-8"},
			},
			body: []byte(`<!doctype html><title>Attention Required</title><script src="/cdn-cgi/challenge-platform/h/b/orchestrate"></script>`),
			want: true,
		},
		{
			name:       "turnstile marker",
			statusCode: http.StatusForbidden,
			body:       []byte(`<html><body><script src="https://challenges.cloudflare.com/turnstile/v0/api.js"></script></body></html>`),
			want:       true,
		},
		{
			name:       "ray header alone is not challenge",
			statusCode: http.StatusForbidden,
			headers: http.Header{
				"Content-Type": []string{"text/html; charset=utf-8"},
				"Cf-Ray":       []string{"abc-SJC"},
			},
			body: []byte(`<html><body>Access denied</body></html>`),
			want: false,
		},
		{
			name:       "server header alone is not challenge",
			statusCode: http.StatusServiceUnavailable,
			headers: http.Header{
				"Content-Type": []string{"text/html"},
				"Server":       []string{"cloudflare"},
			},
			body: []byte(`<html><body>Service unavailable</body></html>`),
			want: false,
		},
		{
			name:       "generic html challenge wording",
			statusCode: http.StatusForbidden,
			headers: http.Header{
				"Content-Type": []string{"text/html"},
			},
			body: []byte(`<html><body>Cloudflare challenge</body></html>`),
			want: true,
		},
		{
			name:       "non challenge status",
			statusCode: http.StatusInternalServerError,
			body:       []byte(`<html><title>Just a moment...</title></html>`),
			want:       false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, IsCloudflareChallengeResponse(tt.statusCode, tt.headers, tt.body))
		})
	}
}
