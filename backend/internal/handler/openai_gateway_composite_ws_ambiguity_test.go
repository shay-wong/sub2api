package handler

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func TestResolveResponsesWebSocketCompositeRoute_RejectsOAuthAmbiguity(t *testing.T) {
	resolver := service.NewCompositeRouteResolver(nil)
	resolver.SetModelOwnershipResolver(func(context.Context, int64, string) (service.CompositeModelOwnership, error) {
		return service.CompositeModelOwnership{Ambiguous: true, OpenAIGatewayOAuthClaimed: true}, nil
	})
	h := &OpenAIGatewayHandler{compositeResolver: resolver}
	apiKey := &service.APIKey{Group: &service.Group{ID: 73, Platform: service.PlatformComposite}}

	ctx, model, matched, err := h.resolveResponsesWebSocketCompositeRoute(context.Background(), apiKey, "gpt-6-astra")
	var closeErr *service.OpenAIWSClientCloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, websocket.StatusPolicyViolation, closeErr.StatusCode())
	require.Contains(t, closeErr.Reason(), "gpt-6-astra")
	require.False(t, matched)
	require.Equal(t, "gpt-6-astra", model)
	_, resolved := service.ResolvedTargetPlatformFromContext(ctx)
	require.False(t, resolved, "OAuth ambiguity must not become a guessed platform")
}
