package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIModelsRefreshRechecksFreshCache(t *testing.T) {
	s := &OpenAIGatewayService{}
	request := openAIModelsRequest{accountID: 1}
	key := buildOpenAIModelsCacheKey(request)
	// A delayed caller already observed a miss; another flight has now completed.
	manifest := &OpenAIModelsResponse{Body: []byte(`{"models":[{"slug":"cached-model"}]}`)}
	s.openAIModelsCache.set(key, manifest, time.Now())
	called := false
	result := <-s.refreshCachedOpenAIModels(key, request, func(context.Context, string) (*OpenAIModelsResponse, error) {
		called = true
		return &OpenAIModelsResponse{}, nil
	})
	require.NoError(t, result.Err)
	require.False(t, called, "a completed refresh must prevent a duplicate upstream call")
	require.Equal(t, manifest, result.Val)
}
