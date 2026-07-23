package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type compositeWSRouteRepoStub struct {
	routes []service.CompositeModelRoute
}

func (s compositeWSRouteRepoStub) ListByGroup(context.Context, int64, bool) ([]service.CompositeModelRoute, error) {
	return append([]service.CompositeModelRoute(nil), s.routes...), nil
}

func (compositeWSRouteRepoStub) Create(context.Context, *service.CompositeModelRoute) error {
	return nil
}
func (compositeWSRouteRepoStub) Update(context.Context, *service.CompositeModelRoute) error {
	return nil
}
func (compositeWSRouteRepoStub) Delete(context.Context, int64) error        { return nil }
func (compositeWSRouteRepoStub) DeleteByGroup(context.Context, int64) error { return nil }

func TestResolveResponsesWebSocketCompositeRoute_ExplicitAliasPreservesPublicModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(73)
	h := &OpenAIGatewayHandler{
		compositeResolver: service.NewCompositeRouteResolver(compositeWSRouteRepoStub{
			routes: []service.CompositeModelRoute{{
				ID:             1,
				GroupID:        groupID,
				PublicModel:    "all/gpt-5",
				MatchType:      service.CompositeRouteMatchExact,
				TargetPlatform: service.PlatformOpenAI,
				UpstreamModel:  "gpt-5",
				Endpoint:       service.CompositeRouteEndpointResponses,
				Enabled:        true,
			}},
		}),
	}
	apiKey := &service.APIKey{
		GroupID: &groupID,
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformComposite,
		},
	}

	ctx, upstreamModel, matched, err := h.resolveResponsesWebSocketCompositeRoute(
		context.Background(),
		apiKey,
		"all/gpt-5",
	)
	require.NoError(t, err)
	require.True(t, matched)
	require.Equal(t, "gpt-5", upstreamModel)

	platform, ok := service.ResolvedTargetPlatformFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, service.PlatformOpenAI, platform)
	resolvedModel, ok := service.ResolvedUpstreamModelFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, "gpt-5", resolvedModel)
	publicModel, ok := service.RequestedPublicModelFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, "all/gpt-5", publicModel)

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest("GET", "/openai/v1/responses", nil).WithContext(ctx)
	usageFields := clientRequestedUsageFields(ginCtx, service.ChannelMappingResult{}, upstreamModel, upstreamModel)
	require.Equal(t, "all/gpt-5", usageFields.OriginalModel)
}
