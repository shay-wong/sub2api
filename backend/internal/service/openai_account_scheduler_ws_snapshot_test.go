//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

type schedulerTestGroupRepo struct {
	groupRepoNoop
	groups map[int64]*Group
	calls  []int64
}

func (r *schedulerTestGroupRepo) GetByID(ctx context.Context, id int64) (*Group, error) {
	r.calls = append(r.calls, id)
	if r.groups == nil {
		return nil, nil
	}
	return r.groups[id], nil
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_UsesWSPassthroughSnapshotFlags(t *testing.T) {
	groupID := int64(10105)
	group := &Group{ID: groupID, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, RateMultiplier: 2}
	ctx := context.WithValue(context.Background(), ctxkey.Group, group)
	account := &Account{
		ID:          35001,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 10,
		Extra: map[string]any{
			"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough,
		},
	}

	snapshotCache := &openAISnapshotCacheStub{
		snapshotAccounts: []*Account{account},
		accountsByID:     map[int64]*Account{account.ID: account},
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool

	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{*account}},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotCache},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"session_hash_ws_passthrough",
		"gpt-5.1",
		nil,
		OpenAIUpstreamTransportResponsesWebsocketV2,
		false,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, account.ID, selection.Account.ID)
	require.NotNil(t, selection.GroupID)
	require.Equal(t, groupID, *selection.GroupID)
	require.Same(t, group, selection.Group)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_ResolvesGroupSnapshotWithoutContext(t *testing.T) {
	groupID := int64(10106)
	group := &Group{ID: groupID, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, RateMultiplier: 2}
	account := &Account{
		ID:          35002,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 10,
		Extra: map[string]any{
			"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough,
		},
	}

	groupRepo := &schedulerTestGroupRepo{groups: map[int64]*Group{groupID: group}}
	snapshotCache := &openAISnapshotCacheStub{
		snapshotAccounts: []*Account{account},
		accountsByID:     map[int64]*Account{account.ID: account},
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool

	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{*account}},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotCache, groupRepo: groupRepo},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		context.Background(),
		&groupID,
		"",
		"session_hash_ws_passthrough_no_context",
		"gpt-5.1",
		nil,
		OpenAIUpstreamTransportResponsesWebsocketV2,
		false,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.GroupID)
	require.Equal(t, groupID, *selection.GroupID)
	require.Same(t, group, selection.Group)
	require.NotEmpty(t, groupRepo.calls)
	require.Equal(t, groupID, groupRepo.calls[0])
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_UsesFallbackGroupSnapshot(t *testing.T) {
	originalGroupID := int64(10107)
	fallbackGroupID := int64(10108)
	originalGroup := &Group{
		ID:              originalGroupID,
		Platform:        PlatformOpenAI,
		Status:          StatusActive,
		Hydrated:        true,
		ClaudeCodeOnly:  true,
		FallbackGroupID: &fallbackGroupID,
	}
	fallbackGroup := &Group{ID: fallbackGroupID, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true}
	account := &Account{
		ID:          35003,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 10,
		Extra: map[string]any{
			"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough,
		},
	}

	groupRepo := &schedulerTestGroupRepo{groups: map[int64]*Group{
		originalGroupID: originalGroup,
		fallbackGroupID: fallbackGroup,
	}}
	snapshotCache := &openAISnapshotCacheStub{
		snapshotAccounts: []*Account{account},
		accountsByID:     map[int64]*Account{account.ID: account},
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool

	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{*account}},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotCache, groupRepo: groupRepo},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		context.Background(),
		&originalGroupID,
		"",
		"session_hash_ws_passthrough_fallback",
		"gpt-5.1",
		nil,
		OpenAIUpstreamTransportResponsesWebsocketV2,
		false,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.GroupID)
	require.Equal(t, fallbackGroupID, *selection.GroupID)
	require.Same(t, fallbackGroup, selection.Group)
	require.GreaterOrEqual(t, len(groupRepo.calls), 2)
	require.Equal(t, originalGroupID, groupRepo.calls[0])
	require.Equal(t, fallbackGroupID, groupRepo.calls[1])
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
}
