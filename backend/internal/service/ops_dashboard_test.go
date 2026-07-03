//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGetDashboardOverview_ProjectScopeUsesProjectBusinessHealth(t *testing.T) {
	t.Parallel()

	start := time.Now().UTC().Add(-1 * time.Hour)
	end := time.Now().UTC()
	failedAt := end.Add(-1 * time.Minute)
	getLatestSystemMetricsCalled := false
	listJobHeartbeatsCalled := false
	repo := &opsRepoMock{
		GetDashboardOverviewFn: func(ctx context.Context, filter *OpsDashboardFilter) (*OpsDashboardOverview, error) {
			return &OpsDashboardOverview{
				StartTime:            filter.StartTime,
				EndTime:              filter.EndTime,
				RequestCountTotal:    100,
				RequestCountSLA:      100,
				SuccessCount:         100,
				SLA:                  1,
				ErrorRate:            0,
				UpstreamErrorRate:    0,
				TTFT:                 OpsPercentiles{P99: intPtr(100)},
				JobHeartbeats:        nil,
				SystemMetrics:        nil,
				ErrorCountTotal:      0,
				ErrorCountSLA:        0,
				BusinessLimitedCount: 0,
			}, nil
		},
		GetLatestSystemMetricsFn: func(ctx context.Context, windowMinutes int) (*OpsSystemMetricsSnapshot, error) {
			getLatestSystemMetricsCalled = true
			return &OpsSystemMetricsSnapshot{
				DBOK:               boolPtr(false),
				RedisOK:            boolPtr(false),
				CPUUsagePercent:    float64Ptr(99),
				MemoryUsagePercent: float64Ptr(99),
			}, nil
		},
		ListJobHeartbeatsFn: func(ctx context.Context) ([]*OpsJobHeartbeat, error) {
			listJobHeartbeatsCalled = true
			return []*OpsJobHeartbeat{{JobName: "global-job", LastErrorAt: &failedAt}}, nil
		},
	}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	out, err := svc.GetDashboardOverview(WithProjectID(context.Background(), 42), &OpsDashboardFilter{
		StartTime: start,
		EndTime:   end,
		QueryMode: OpsQueryModeRaw,
	})

	require.NoError(t, err)
	require.Equal(t, 100, out.HealthScore)
	require.Nil(t, out.SystemMetrics)
	require.Empty(t, out.JobHeartbeats)
	require.False(t, getLatestSystemMetricsCalled)
	require.False(t, listJobHeartbeatsCalled)
}

func TestGetDashboardOverview_GlobalScopeIncludesRuntimeHealth(t *testing.T) {
	t.Parallel()

	start := time.Now().UTC().Add(-1 * time.Hour)
	end := time.Now().UTC()
	failedAt := end.Add(-1 * time.Minute)
	listJobHeartbeatsCalled := false
	repo := &opsRepoMock{
		GetDashboardOverviewFn: func(ctx context.Context, filter *OpsDashboardFilter) (*OpsDashboardOverview, error) {
			return &OpsDashboardOverview{
				StartTime:         filter.StartTime,
				EndTime:           filter.EndTime,
				RequestCountTotal: 100,
				RequestCountSLA:   100,
				SuccessCount:      100,
				SLA:               1,
				ErrorRate:         0,
				UpstreamErrorRate: 0,
				TTFT:              OpsPercentiles{P99: intPtr(100)},
			}, nil
		},
		GetLatestSystemMetricsFn: func(ctx context.Context, windowMinutes int) (*OpsSystemMetricsSnapshot, error) {
			return &OpsSystemMetricsSnapshot{
				DBOK:               boolPtr(false),
				RedisOK:            boolPtr(false),
				CPUUsagePercent:    float64Ptr(99),
				MemoryUsagePercent: float64Ptr(99),
			}, nil
		},
		ListJobHeartbeatsFn: func(ctx context.Context) ([]*OpsJobHeartbeat, error) {
			listJobHeartbeatsCalled = true
			return []*OpsJobHeartbeat{{JobName: "global-job", LastErrorAt: &failedAt}}, nil
		},
	}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	out, err := svc.GetDashboardOverview(context.Background(), &OpsDashboardFilter{
		StartTime: start,
		EndTime:   end,
		QueryMode: OpsQueryModeRaw,
	})

	require.NoError(t, err)
	require.True(t, listJobHeartbeatsCalled)
	require.NotEmpty(t, out.JobHeartbeats)
	require.Less(t, out.HealthScore, 100)
}
