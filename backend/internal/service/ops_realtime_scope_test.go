package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type opsRealtimeScopeAccountRepo struct {
	AccountRepository
	accounts []Account
}

func (r *opsRealtimeScopeAccountRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string) ([]Account, *pagination.PaginationResult, error) {
	out := make([]Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		if platform != "" && account.Platform != platform {
			continue
		}
		if groupID > 0 && !opsRealtimeScopeAccountHasGroup(account, groupID) {
			continue
		}
		out = append(out, account)
	}
	return out, &pagination.PaginationResult{Total: int64(len(out)), Page: params.Page, PageSize: params.PageSize}, nil
}

func (r *opsRealtimeScopeAccountRepo) ListWithGroupScope(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupIDs []int64, privacyMode string) ([]Account, *pagination.PaginationResult, error) {
	out := make([]Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		if platform != "" && account.Platform != platform {
			continue
		}
		if !opsRealtimeScopeAccountMatchesAnyGroup(account, groupIDs) {
			continue
		}
		out = append(out, account)
	}
	return out, &pagination.PaginationResult{Total: int64(len(out)), Page: params.Page, PageSize: params.PageSize}, nil
}

func opsRealtimeScopeAccountMatchesAnyGroup(account Account, groupIDs []int64) bool {
	for _, id := range groupIDs {
		if opsRealtimeScopeAccountHasGroup(account, id) {
			return true
		}
	}
	return false
}

func opsRealtimeScopeAccountHasGroup(account Account, groupID int64) bool {
	for _, id := range account.GroupIDs {
		if id == groupID {
			return true
		}
	}
	for _, group := range account.Groups {
		if group != nil && group.ID == groupID {
			return true
		}
	}
	for _, accountGroup := range account.AccountGroups {
		if accountGroup.GroupID == groupID {
			return true
		}
	}
	return false
}

func TestOpsServiceRealtimeStatsTrimGroupsToOperatorScope(t *testing.T) {
	visible := &Group{ID: 10, Name: "visible", Platform: PlatformOpenAI, Status: StatusActive}
	hidden := &Group{ID: 30, Name: "hidden", Platform: PlatformOpenAI, Status: StatusActive}
	repo := &opsRealtimeScopeAccountRepo{accounts: []Account{
		{
			ID:          1,
			Name:        "shared",
			Platform:    PlatformOpenAI,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 4,
			GroupIDs:    []int64{30, 10},
			Groups:      []*Group{hidden, visible},
			AccountGroups: []AccountGroup{
				{AccountID: 1, GroupID: 30, Group: hidden},
				{AccountID: 1, GroupID: 10, Group: visible},
			},
		},
	}}
	svc := NewOpsService(nil, nil, nil, repo, nil, nil, nil, nil, nil, nil, nil)

	_, concurrencyGroups, concurrencyAccounts, _, err := svc.GetConcurrencyStats(context.Background(), "", nil, 10)
	require.NoError(t, err)
	require.Contains(t, concurrencyGroups, int64(10))
	require.NotContains(t, concurrencyGroups, int64(30))
	require.Equal(t, int64(10), concurrencyAccounts[1].GroupID)
	require.Equal(t, "visible", concurrencyAccounts[1].GroupName)

	_, availabilityGroups, availabilityAccounts, _, err := svc.GetAccountAvailabilityStats(context.Background(), "", nil, 10)
	require.NoError(t, err)
	require.Contains(t, availabilityGroups, int64(10))
	require.NotContains(t, availabilityGroups, int64(30))
	require.Equal(t, int64(10), availabilityAccounts[1].GroupID)
	require.Equal(t, "visible", availabilityAccounts[1].GroupName)
}

func TestOpsServiceWindowStatsAppliesOperatorGroupScope(t *testing.T) {
	var captured *OpsDashboardFilter
	repo := &opsRepoMock{
		GetWindowStatsFn: func(ctx context.Context, filter *OpsDashboardFilter) (*OpsWindowStats, error) {
			cloned := *filter
			cloned.GroupIDs = append([]int64(nil), filter.GroupIDs...)
			captured = &cloned
			return &OpsWindowStats{SuccessCount: 1}, nil
		},
	}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	start := time.Now().UTC().Add(-time.Minute)
	end := time.Now().UTC()

	stats, err := svc.GetWindowStatsWithGroupScope(context.Background(), start, end, []int64{20, 10, 10, 0}, false)

	require.NoError(t, err)
	require.NotNil(t, stats)
	require.NotNil(t, captured)
	require.ElementsMatch(t, []int64{10, 20}, captured.GroupIDs)
	require.False(t, captured.GroupScopeEmpty)
}

func TestOpsServiceWindowStatsKeepsEmptyOperatorScope(t *testing.T) {
	var captured *OpsDashboardFilter
	repo := &opsRepoMock{
		GetWindowStatsFn: func(ctx context.Context, filter *OpsDashboardFilter) (*OpsWindowStats, error) {
			cloned := *filter
			captured = &cloned
			return &OpsWindowStats{}, nil
		},
	}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	start := time.Now().UTC().Add(-time.Minute)
	end := time.Now().UTC()

	_, err := svc.GetWindowStatsWithGroupScope(context.Background(), start, end, nil, true)

	require.NoError(t, err)
	require.NotNil(t, captured)
	require.Empty(t, captured.GroupIDs)
	require.True(t, captured.GroupScopeEmpty)
}

func TestOpsServiceUsageRecordRuntimeStatsAreProcessScoped(t *testing.T) {
	svc := NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	pool := NewUsageRecordWorkerPoolWithOptions(UsageRecordWorkerPoolOptions{
		WorkerCount:           1,
		QueueSize:             7,
		TaskTimeout:           2 * time.Second,
		OverflowPolicy:        config.UsageRecordOverflowPolicySync,
		OverflowSamplePercent: 0,
		AutoScaleEnabled:      false,
	})
	t.Cleanup(pool.Stop)
	svc.SetUsageRecordWorkerPool(pool)

	stats, err := svc.GetUsageRecordRuntimeStats(context.Background())

	require.NoError(t, err)
	require.Equal(t, "process", stats.Scope)
	require.NotNil(t, stats.WorkerPool)
	require.Equal(t, 7, stats.WorkerPool.QueueSize)
	require.Equal(t, config.UsageRecordOverflowPolicySync, stats.WorkerPool.OverflowPolicy)
	require.False(t, stats.ProcessStartedAt.IsZero())
	require.GreaterOrEqual(t, stats.UptimeSeconds, int64(0))
	require.GreaterOrEqual(t, stats.Persistence.PostUsageBillingTimeoutSeconds, int64(1))
}
