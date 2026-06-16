package admin

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (h *DashboardHandler) scopedDashboardGroupIDs(scope *adminAccessScope, groupID int64) ([]int64, error) {
	return scope.dashboardGroupIDs(groupID)
}

func (h *DashboardHandler) getUsageTrendForScope(
	ctx context.Context,
	scope *adminAccessScope,
	startTime, endTime time.Time,
	granularity string,
	userID, apiKeyID, accountID, groupID int64,
	model string,
	requestType *int16,
	stream *bool,
	billingType *int8,
) ([]usagestats.TrendDataPoint, bool, error) {
	groupIDs, err := h.scopedDashboardGroupIDs(scope, groupID)
	if err != nil {
		return nil, false, err
	}
	if !scope.isScoped() {
		return h.getUsageTrendCached(ctx, startTime, endTime, granularity, userID, apiKeyID, accountID, groupID, model, requestType, stream, billingType)
	}
	if len(groupIDs) == 0 {
		return []usagestats.TrendDataPoint{}, false, nil
	}
	byDate := map[string]*usagestats.TrendDataPoint{}
	for _, scopedGroupID := range groupIDs {
		points, err := h.dashboardService.GetUsageTrendWithFilters(ctx, startTime, endTime, granularity, userID, apiKeyID, accountID, scopedGroupID, model, requestType, stream, billingType)
		if err != nil {
			return nil, false, err
		}
		for _, point := range points {
			dst := byDate[point.Date]
			if dst == nil {
				copy := point
				byDate[point.Date] = &copy
				continue
			}
			dst.Requests += point.Requests
			dst.InputTokens += point.InputTokens
			dst.OutputTokens += point.OutputTokens
			dst.CacheCreationTokens += point.CacheCreationTokens
			dst.CacheReadTokens += point.CacheReadTokens
			dst.TotalTokens += point.TotalTokens
			dst.Cost += point.Cost
			dst.ActualCost += point.ActualCost
		}
	}
	return trendMapToSortedSlice(byDate), false, nil
}

func (h *DashboardHandler) getModelStatsForScope(
	ctx context.Context,
	scope *adminAccessScope,
	startTime, endTime time.Time,
	userID, apiKeyID, accountID, groupID int64,
	modelSource string,
	requestType *int16,
	stream *bool,
	billingType *int8,
) ([]usagestats.ModelStat, bool, error) {
	groupIDs, err := h.scopedDashboardGroupIDs(scope, groupID)
	if err != nil {
		return nil, false, err
	}
	if !scope.isScoped() {
		return h.getModelStatsCached(ctx, startTime, endTime, userID, apiKeyID, accountID, groupID, modelSource, requestType, stream, billingType)
	}
	if len(groupIDs) == 0 {
		return []usagestats.ModelStat{}, false, nil
	}
	byModel := map[string]*usagestats.ModelStat{}
	for _, scopedGroupID := range groupIDs {
		rows, err := h.dashboardService.GetModelStatsWithFiltersBySource(ctx, startTime, endTime, userID, apiKeyID, accountID, scopedGroupID, requestType, stream, billingType, modelSource)
		if err != nil {
			return nil, false, err
		}
		for _, row := range rows {
			dst := byModel[row.Model]
			if dst == nil {
				copy := row
				byModel[row.Model] = &copy
				continue
			}
			dst.Requests += row.Requests
			dst.InputTokens += row.InputTokens
			dst.OutputTokens += row.OutputTokens
			dst.CacheCreationTokens += row.CacheCreationTokens
			dst.CacheReadTokens += row.CacheReadTokens
			dst.TotalTokens += row.TotalTokens
			dst.Cost += row.Cost
			dst.ActualCost += row.ActualCost
			dst.AccountCost += row.AccountCost
		}
	}
	return modelMapToSortedSlice(byModel), false, nil
}

func (h *DashboardHandler) getGroupStatsForScope(
	ctx context.Context,
	scope *adminAccessScope,
	startTime, endTime time.Time,
	userID, apiKeyID, accountID, groupID int64,
	requestType *int16,
	stream *bool,
	billingType *int8,
) ([]usagestats.GroupStat, bool, error) {
	groupIDs, err := h.scopedDashboardGroupIDs(scope, groupID)
	if err != nil {
		return nil, false, err
	}
	if !scope.isScoped() {
		return h.getGroupStatsCached(ctx, startTime, endTime, userID, apiKeyID, accountID, groupID, requestType, stream, billingType)
	}
	if len(groupIDs) == 0 {
		return []usagestats.GroupStat{}, false, nil
	}
	out := make([]usagestats.GroupStat, 0, len(groupIDs))
	for _, scopedGroupID := range groupIDs {
		rows, err := h.dashboardService.GetGroupStatsWithFilters(ctx, startTime, endTime, userID, apiKeyID, accountID, scopedGroupID, requestType, stream, billingType)
		if err != nil {
			return nil, false, err
		}
		out = append(out, rows...)
	}
	return out, false, nil
}

func (h *DashboardHandler) getDashboardStatsForScope(ctx context.Context, scope *adminAccessScope, startTime, endTime time.Time) (*usagestats.DashboardStats, error) {
	if scope == nil || !scope.isScoped() {
		return h.dashboardService.GetDashboardStats(ctx)
	}
	if len(scope.GroupIDs) == 0 {
		return &usagestats.DashboardStats{StatsStale: true}, nil
	}
	var merged usagestats.DashboardStats
	useAccountStats := false
	if h.adminService != nil {
		stats, ok, err := h.getScopedAccountStats(ctx, scope.GroupIDs)
		if err != nil {
			return nil, err
		}
		if ok {
			useAccountStats = true
			merged.TotalAccounts = stats.TotalAccounts
			merged.NormalAccounts = stats.NormalAccounts
			merged.ErrorAccounts = stats.ErrorAccounts
			merged.RateLimitAccounts = stats.RateLimitAccounts
			merged.OverloadAccounts = stats.OverloadAccounts
		}
	}
	for _, groupID := range scope.GroupIDs {
		stats, err := h.dashboardService.GetUsageTrendWithFilters(ctx, startTime, endTime, "day", 0, 0, 0, groupID, "", nil, nil, nil)
		if err != nil {
			return nil, err
		}
		for _, point := range stats {
			merged.TotalRequests += point.Requests
			merged.TotalInputTokens += point.InputTokens
			merged.TotalOutputTokens += point.OutputTokens
			merged.TotalCacheCreationTokens += point.CacheCreationTokens
			merged.TotalCacheReadTokens += point.CacheReadTokens
			merged.TotalCost += point.Cost
			merged.TotalActualCost += point.ActualCost
		}
		todayStart := timezone.Today()
		todayTrend, err := h.dashboardService.GetUsageTrendWithFilters(ctx, todayStart, todayStart.Add(24*time.Hour), "day", 0, 0, 0, groupID, "", nil, nil, nil)
		if err != nil {
			return nil, err
		}
		for _, point := range todayTrend {
			merged.TodayRequests += point.Requests
			merged.TodayInputTokens += point.InputTokens
			merged.TodayOutputTokens += point.OutputTokens
			merged.TodayCacheCreationTokens += point.CacheCreationTokens
			merged.TodayCacheReadTokens += point.CacheReadTokens
			merged.TodayCost += point.Cost
			merged.TodayActualCost += point.ActualCost
		}
		if !useAccountStats {
			groupStats, err := h.dashboardService.GetGroupStatsWithFilters(ctx, startTime, endTime, 0, 0, 0, groupID, nil, nil, nil)
			if err != nil {
				return nil, err
			}
			if len(groupStats) > 0 {
				merged.TotalAccounts++
			}
		}
	}
	merged.TotalTokens = merged.TotalInputTokens + merged.TotalOutputTokens + merged.TotalCacheCreationTokens + merged.TotalCacheReadTokens
	merged.TodayTokens = merged.TodayInputTokens + merged.TodayOutputTokens + merged.TodayCacheCreationTokens + merged.TodayCacheReadTokens
	merged.StatsStale = true
	return &merged, nil
}

func (h *DashboardHandler) getScopedAccountStats(ctx context.Context, groupIDs []int64) (*usagestats.DashboardStats, bool, error) {
	stats := &usagestats.DashboardStats{}
	if h.adminService == nil || len(groupIDs) == 0 {
		return stats, true, nil
	}
	scopedList, ok := h.adminService.(interface {
		ListAccountsByGroupScope(context.Context, int, int, string, string, string, string, []int64, string, string, string) ([]service.Account, int64, error)
	})
	if !ok {
		return stats, false, nil
	}
	now := time.Now()
	seen := make(map[int64]struct{})
	const pageSize = 500
	for page := 1; ; page++ {
		accounts, total, err := scopedList.ListAccountsByGroupScope(ctx, page, pageSize, "", "", "", "", groupIDs, "", "id", "asc")
		if err != nil {
			return nil, true, err
		}
		for i := range accounts {
			account := accounts[i]
			if account.ID <= 0 {
				continue
			}
			if _, ok := seen[account.ID]; ok {
				continue
			}
			seen[account.ID] = struct{}{}
			stats.TotalAccounts++
			if account.Status == service.StatusActive && account.Schedulable {
				stats.NormalAccounts++
			}
			if account.Status == service.StatusError {
				stats.ErrorAccounts++
			}
			if account.RateLimitedAt != nil && account.RateLimitResetAt != nil && account.RateLimitResetAt.After(now) {
				stats.RateLimitAccounts++
			}
			if account.OverloadUntil != nil && account.OverloadUntil.After(now) {
				stats.OverloadAccounts++
			}
		}
		if len(accounts) < pageSize || (total > 0 && int64(len(seen)) >= total) {
			break
		}
	}
	return stats, true, nil
}

func trendMapToSortedSlice(values map[string]*usagestats.TrendDataPoint) []usagestats.TrendDataPoint {
	out := make([]usagestats.TrendDataPoint, 0, len(values))
	for _, value := range values {
		out = append(out, *value)
	}
	sortTrendPoints(out)
	return out
}

func modelMapToSortedSlice(values map[string]*usagestats.ModelStat) []usagestats.ModelStat {
	out := make([]usagestats.ModelStat, 0, len(values))
	for _, value := range values {
		out = append(out, *value)
	}
	sortModelStats(out)
	return out
}

func sortTrendPoints(points []usagestats.TrendDataPoint) {
	for i := 1; i < len(points); i++ {
		for j := i; j > 0 && points[j-1].Date > points[j].Date; j-- {
			points[j-1], points[j] = points[j], points[j-1]
		}
	}
}

func sortModelStats(rows []usagestats.ModelStat) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j-1].TotalTokens < rows[j].TotalTokens; j-- {
			rows[j-1], rows[j] = rows[j], rows[j-1]
		}
	}
}

var _ = service.AdminPermissionDashboardRead
