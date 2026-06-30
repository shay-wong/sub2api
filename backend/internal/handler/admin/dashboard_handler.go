package admin

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// DashboardHandler handles admin dashboard statistics
type DashboardHandler struct {
	dashboardService   *service.DashboardService
	aggregationService *service.DashboardAggregationService
	permissionService  *service.PermissionService
	adminService       service.AdminService
	startTime          time.Time // Server start time for uptime calculation
}

// NewDashboardHandler creates a new admin dashboard handler
func NewDashboardHandler(dashboardService *service.DashboardService, aggregationService *service.DashboardAggregationService, permissionService ...*service.PermissionService) *DashboardHandler {
	var perm *service.PermissionService
	if len(permissionService) > 0 {
		perm = permissionService[0]
	}
	return &DashboardHandler{
		dashboardService:   dashboardService,
		aggregationService: aggregationService,
		permissionService:  perm,
		startTime:          time.Now(),
	}
}

func (h *DashboardHandler) SetAdminService(adminService service.AdminService) {
	h.adminService = adminService
}

// parseTimeRange parses start_date, end_date query parameters
// Uses user's timezone if provided, otherwise falls back to server timezone
func parseTimeRange(c *gin.Context) (time.Time, time.Time) {
	userTZ := c.Query("timezone") // Get user's timezone from request
	now := timezone.NowInUserLocation(userTZ)
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	var startTime, endTime time.Time

	if startDate != "" {
		if t, err := timezone.ParseInUserLocation("2006-01-02", startDate, userTZ); err == nil {
			startTime = t
		} else {
			startTime = timezone.StartOfDayInUserLocation(now.AddDate(0, 0, -7), userTZ)
		}
	} else {
		startTime = timezone.StartOfDayInUserLocation(now.AddDate(0, 0, -7), userTZ)
	}

	if endDate != "" {
		if t, err := timezone.ParseInUserLocation("2006-01-02", endDate, userTZ); err == nil {
			endTime = t.AddDate(0, 0, 1) // Include the end date using calendar-day math for DST.
		} else {
			endTime = timezone.StartOfDayInUserLocation(now.AddDate(0, 0, 1), userTZ)
		}
	} else {
		endTime = timezone.StartOfDayInUserLocation(now.AddDate(0, 0, 1), userTZ)
	}

	return startTime, endTime
}

func dashboardResponseEndDate(c *gin.Context, endTime time.Time) string {
	if endDate := strings.TrimSpace(c.Query("end_date")); endDate != "" {
		if _, err := timezone.ParseInUserLocation("2006-01-02", endDate, c.Query("timezone")); err == nil {
			return endDate
		}
	}
	return endTime.AddDate(0, 0, -1).Format("2006-01-02")
}

func parseDashboardModelFilterSource(c *gin.Context) (string, bool) {
	raw := strings.TrimSpace(c.DefaultQuery("model_filter_source", usagestats.ModelSourceRequested))
	if !usagestats.IsValidModelSource(raw) {
		response.BadRequest(c, "Invalid model_filter_source, use requested/upstream/mapping")
		return "", false
	}
	return raw, true
}

// GetStats handles getting dashboard statistics
// GET /api/v1/admin/dashboard/stats
func (h *DashboardHandler) GetStats(c *gin.Context) {
	stats, err := h.getDashboardStatsForRequest(c)
	if err != nil {
		response.Error(c, 500, "Failed to get dashboard statistics")
		return
	}

	// Calculate uptime in seconds
	uptime := int64(time.Since(h.startTime).Seconds())

	response.Success(c, gin.H{
		// 用户统计
		"total_users":     stats.TotalUsers,
		"today_new_users": stats.TodayNewUsers,
		"active_users":    stats.ActiveUsers,

		// API Key 统计
		"total_api_keys":  stats.TotalAPIKeys,
		"active_api_keys": stats.ActiveAPIKeys,

		// 账户统计
		"total_accounts":     stats.TotalAccounts,
		"normal_accounts":    stats.NormalAccounts,
		"error_accounts":     stats.ErrorAccounts,
		"ratelimit_accounts": stats.RateLimitAccounts,
		"overload_accounts":  stats.OverloadAccounts,

		// 累计 Token 使用统计
		"total_requests":              stats.TotalRequests,
		"total_input_tokens":          stats.TotalInputTokens,
		"total_output_tokens":         stats.TotalOutputTokens,
		"total_cache_creation_tokens": stats.TotalCacheCreationTokens,
		"total_cache_read_tokens":     stats.TotalCacheReadTokens,
		"total_tokens":                stats.TotalTokens,
		"total_cost":                  stats.TotalCost,       // 标准计费
		"total_actual_cost":           stats.TotalActualCost, // 实际扣除

		// 今日 Token 使用统计
		"today_requests":              stats.TodayRequests,
		"today_input_tokens":          stats.TodayInputTokens,
		"today_output_tokens":         stats.TodayOutputTokens,
		"today_cache_creation_tokens": stats.TodayCacheCreationTokens,
		"today_cache_read_tokens":     stats.TodayCacheReadTokens,
		"today_tokens":                stats.TodayTokens,
		"today_cost":                  stats.TodayCost,       // 今日标准计费
		"today_actual_cost":           stats.TodayActualCost, // 今日实际扣除

		// 系统运行统计
		"average_duration_ms": stats.AverageDurationMs,
		"uptime":              uptime,

		// 性能指标
		"rpm": stats.Rpm,
		"tpm": stats.Tpm,

		// 预聚合新鲜度
		"hourly_active_users": stats.HourlyActiveUsers,
		"stats_updated_at":    stats.StatsUpdatedAt,
		"stats_stale":         stats.StatsStale,
	})
}

func (h *DashboardHandler) getDashboardStatsForRequest(c *gin.Context) (*usagestats.DashboardStats, error) {
	scope, err := resolveAdminAccessScope(c, h.permissionService)
	if err != nil {
		return nil, err
	}
	if !scope.isScoped() {
		return h.dashboardService.GetDashboardStats(c.Request.Context())
	}
	startTime, endTime := parseTimeRange(c)
	return h.getDashboardStatsForScope(c.Request.Context(), scope, startTime, endTime)
}

type DashboardAggregationBackfillRequest struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// BackfillAggregation handles triggering aggregation backfill
// POST /api/v1/admin/dashboard/aggregation/backfill
func (h *DashboardHandler) BackfillAggregation(c *gin.Context) {
	if h.aggregationService == nil {
		response.InternalError(c, "Aggregation service not available")
		return
	}

	var req DashboardAggregationBackfillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	start, err := time.Parse(time.RFC3339, req.Start)
	if err != nil {
		response.BadRequest(c, "Invalid start time")
		return
	}
	end, err := time.Parse(time.RFC3339, req.End)
	if err != nil {
		response.BadRequest(c, "Invalid end time")
		return
	}

	if err := h.aggregationService.TriggerBackfill(start, end); err != nil {
		if errors.Is(err, service.ErrDashboardBackfillDisabled) {
			response.Forbidden(c, "Backfill is disabled")
			return
		}
		if errors.Is(err, service.ErrDashboardBackfillTooLarge) {
			response.BadRequest(c, "Backfill range too large")
			return
		}
		response.InternalError(c, "Failed to trigger backfill")
		return
	}

	response.Success(c, gin.H{
		"status": "accepted",
	})
}

// GetRealtimeMetrics handles getting real-time system metrics
// GET /api/v1/admin/dashboard/realtime
func (h *DashboardHandler) GetRealtimeMetrics(c *gin.Context) {
	// Return mock data for now
	response.Success(c, gin.H{
		"active_requests":       0,
		"requests_per_minute":   0,
		"average_response_time": 0,
		"error_rate":            0.0,
	})
}

// GetUsageTrend handles getting usage trend data
// GET /api/v1/admin/dashboard/trend
// Query params: start_date, end_date (YYYY-MM-DD), granularity (day/hour), user_id, api_key_id, model, model_filter_source, account_id, group_id, request_type, stream, billing_type, billing_mode
func (h *DashboardHandler) GetUsageTrend(c *gin.Context) {
	scope, scopeErr := resolveAdminAccessScope(c, h.permissionService)
	if scopeErr != nil {
		response.ErrorFrom(c, scopeErr)
		return
	}
	startTime, endTime := parseTimeRange(c)
	granularity := c.DefaultQuery("granularity", "day")

	// Parse optional filter params
	var userID, apiKeyID, accountID, groupID int64
	var model string
	modelFilterSource, ok := parseDashboardModelFilterSource(c)
	if !ok {
		return
	}
	var requestType *int16
	var stream *bool
	var billingType *int8
	var billingMode string

	if userIDStr := c.Query("user_id"); userIDStr != "" {
		if id, err := strconv.ParseInt(userIDStr, 10, 64); err == nil {
			userID = id
		}
	}
	if apiKeyIDStr := c.Query("api_key_id"); apiKeyIDStr != "" {
		if id, err := strconv.ParseInt(apiKeyIDStr, 10, 64); err == nil {
			apiKeyID = id
		}
	}
	if accountIDStr := c.Query("account_id"); accountIDStr != "" {
		if id, err := strconv.ParseInt(accountIDStr, 10, 64); err == nil {
			accountID = id
		}
	}
	if groupIDStr := c.Query("group_id"); groupIDStr != "" {
		if id, err := strconv.ParseInt(groupIDStr, 10, 64); err == nil {
			groupID = id
		}
	}
	if modelStr := c.Query("model"); modelStr != "" {
		model = modelStr
	}
	if requestTypeStr := strings.TrimSpace(c.Query("request_type")); requestTypeStr != "" {
		parsed, err := service.ParseUsageRequestType(requestTypeStr)
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		value := int16(parsed)
		requestType = &value
	} else if streamStr := c.Query("stream"); streamStr != "" {
		if streamVal, err := strconv.ParseBool(streamStr); err == nil {
			stream = &streamVal
		} else {
			response.BadRequest(c, "Invalid stream value, use true or false")
			return
		}
	}
	if billingTypeStr := c.Query("billing_type"); billingTypeStr != "" {
		if v, err := strconv.ParseInt(billingTypeStr, 10, 8); err == nil {
			bt := int8(v)
			billingType = &bt
		} else {
			response.BadRequest(c, "Invalid billing_type")
			return
		}
	}
	if billingModeStr := strings.TrimSpace(c.Query("billing_mode")); billingModeStr != "" {
		if !service.BillingMode(billingModeStr).IsValid() {
			response.BadRequest(c, "Invalid billing_mode")
			return
		}
		billingMode = billingModeStr
	}

	filters := usagestats.UsageLogFilters{
		UserID:            userID,
		APIKeyID:          apiKeyID,
		AccountID:         accountID,
		GroupID:           groupID,
		Model:             model,
		ModelFilterSource: modelFilterSource,
		RequestType:       requestType,
		Stream:            stream,
		BillingType:       billingType,
		BillingMode:       billingMode,
	}
	trend, hit, err := h.getUsageTrendForScope(c.Request.Context(), scope, startTime, endTime, granularity, filters)
	if err != nil {
		response.Error(c, 500, "Failed to get usage trend")
		return
	}
	c.Header("X-Snapshot-Cache", cacheStatusValue(hit))

	response.Success(c, gin.H{
		"trend":       trend,
		"start_date":  startTime.Format("2006-01-02"),
		"end_date":    dashboardResponseEndDate(c, endTime),
		"granularity": granularity,
	})
}

// GetModelStats handles getting model usage statistics
// GET /api/v1/admin/dashboard/models
// Query params: start_date, end_date (YYYY-MM-DD), user_id, api_key_id, account_id, group_id, model, model_source, model_filter_source, request_type, stream, billing_type, billing_mode
func (h *DashboardHandler) GetModelStats(c *gin.Context) {
	scope, scopeErr := resolveAdminAccessScope(c, h.permissionService)
	if scopeErr != nil {
		response.ErrorFrom(c, scopeErr)
		return
	}
	startTime, endTime := parseTimeRange(c)
	modelFilterSource, ok := parseDashboardModelFilterSource(c)
	if !ok {
		return
	}

	// Parse optional filter params
	var userID, apiKeyID, accountID, groupID int64
	modelSource := usagestats.ModelSourceRequested
	var model string
	var requestType *int16
	var stream *bool
	var billingType *int8
	var billingMode string

	if userIDStr := c.Query("user_id"); userIDStr != "" {
		if id, err := strconv.ParseInt(userIDStr, 10, 64); err == nil {
			userID = id
		}
	}
	if apiKeyIDStr := c.Query("api_key_id"); apiKeyIDStr != "" {
		if id, err := strconv.ParseInt(apiKeyIDStr, 10, 64); err == nil {
			apiKeyID = id
		}
	}
	if accountIDStr := c.Query("account_id"); accountIDStr != "" {
		if id, err := strconv.ParseInt(accountIDStr, 10, 64); err == nil {
			accountID = id
		}
	}
	if groupIDStr := c.Query("group_id"); groupIDStr != "" {
		if id, err := strconv.ParseInt(groupIDStr, 10, 64); err == nil {
			groupID = id
		}
	}
	if modelStr := c.Query("model"); modelStr != "" {
		model = modelStr
	}
	if rawModelSource := strings.TrimSpace(c.Query("model_source")); rawModelSource != "" {
		if !usagestats.IsValidModelSource(rawModelSource) {
			response.BadRequest(c, "Invalid model_source, use requested/upstream/mapping")
			return
		}
		modelSource = rawModelSource
	}
	if requestTypeStr := strings.TrimSpace(c.Query("request_type")); requestTypeStr != "" {
		parsed, err := service.ParseUsageRequestType(requestTypeStr)
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		value := int16(parsed)
		requestType = &value
	} else if streamStr := c.Query("stream"); streamStr != "" {
		if streamVal, err := strconv.ParseBool(streamStr); err == nil {
			stream = &streamVal
		} else {
			response.BadRequest(c, "Invalid stream value, use true or false")
			return
		}
	}
	if billingTypeStr := c.Query("billing_type"); billingTypeStr != "" {
		if v, err := strconv.ParseInt(billingTypeStr, 10, 8); err == nil {
			bt := int8(v)
			billingType = &bt
		} else {
			response.BadRequest(c, "Invalid billing_type")
			return
		}
	}
	if billingModeStr := strings.TrimSpace(c.Query("billing_mode")); billingModeStr != "" {
		if !service.BillingMode(billingModeStr).IsValid() {
			response.BadRequest(c, "Invalid billing_mode")
			return
		}
		billingMode = billingModeStr
	}

	filters := usagestats.UsageLogFilters{
		UserID:            userID,
		APIKeyID:          apiKeyID,
		AccountID:         accountID,
		GroupID:           groupID,
		Model:             model,
		ModelFilterSource: modelFilterSource,
		RequestType:       requestType,
		Stream:            stream,
		BillingType:       billingType,
		BillingMode:       billingMode,
	}
	stats, hit, err := h.getModelStatsForScope(c.Request.Context(), scope, startTime, endTime, filters, modelSource)
	if err != nil {
		response.Error(c, 500, "Failed to get model statistics")
		return
	}
	c.Header("X-Snapshot-Cache", cacheStatusValue(hit))

	response.Success(c, gin.H{
		"models":     stats,
		"start_date": startTime.Format("2006-01-02"),
		"end_date":   dashboardResponseEndDate(c, endTime),
	})
}

// GetGroupStats handles getting group usage statistics
// GET /api/v1/admin/dashboard/groups
// Query params: start_date, end_date (YYYY-MM-DD), user_id, api_key_id, account_id, group_id, model, model_filter_source, request_type, stream, billing_type, billing_mode
func (h *DashboardHandler) GetGroupStats(c *gin.Context) {
	scope, scopeErr := resolveAdminAccessScope(c, h.permissionService)
	if scopeErr != nil {
		response.ErrorFrom(c, scopeErr)
		return
	}
	startTime, endTime := parseTimeRange(c)
	modelFilterSource, ok := parseDashboardModelFilterSource(c)
	if !ok {
		return
	}

	var userID, apiKeyID, accountID, groupID int64
	var model string
	var requestType *int16
	var stream *bool
	var billingType *int8
	var billingMode string

	if userIDStr := c.Query("user_id"); userIDStr != "" {
		if id, err := strconv.ParseInt(userIDStr, 10, 64); err == nil {
			userID = id
		}
	}
	if apiKeyIDStr := c.Query("api_key_id"); apiKeyIDStr != "" {
		if id, err := strconv.ParseInt(apiKeyIDStr, 10, 64); err == nil {
			apiKeyID = id
		}
	}
	if accountIDStr := c.Query("account_id"); accountIDStr != "" {
		if id, err := strconv.ParseInt(accountIDStr, 10, 64); err == nil {
			accountID = id
		}
	}
	if groupIDStr := c.Query("group_id"); groupIDStr != "" {
		if id, err := strconv.ParseInt(groupIDStr, 10, 64); err == nil {
			groupID = id
		}
	}
	if modelStr := c.Query("model"); modelStr != "" {
		model = modelStr
	}
	if requestTypeStr := strings.TrimSpace(c.Query("request_type")); requestTypeStr != "" {
		parsed, err := service.ParseUsageRequestType(requestTypeStr)
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		value := int16(parsed)
		requestType = &value
	} else if streamStr := c.Query("stream"); streamStr != "" {
		if streamVal, err := strconv.ParseBool(streamStr); err == nil {
			stream = &streamVal
		} else {
			response.BadRequest(c, "Invalid stream value, use true or false")
			return
		}
	}
	if billingTypeStr := c.Query("billing_type"); billingTypeStr != "" {
		if v, err := strconv.ParseInt(billingTypeStr, 10, 8); err == nil {
			bt := int8(v)
			billingType = &bt
		} else {
			response.BadRequest(c, "Invalid billing_type")
			return
		}
	}
	if billingModeStr := strings.TrimSpace(c.Query("billing_mode")); billingModeStr != "" {
		if !service.BillingMode(billingModeStr).IsValid() {
			response.BadRequest(c, "Invalid billing_mode")
			return
		}
		billingMode = billingModeStr
	}

	filters := usagestats.UsageLogFilters{
		UserID:            userID,
		APIKeyID:          apiKeyID,
		AccountID:         accountID,
		GroupID:           groupID,
		Model:             model,
		ModelFilterSource: modelFilterSource,
		RequestType:       requestType,
		Stream:            stream,
		BillingType:       billingType,
		BillingMode:       billingMode,
	}
	stats, hit, err := h.getGroupStatsForScope(c.Request.Context(), scope, startTime, endTime, filters)
	if err != nil {
		response.Error(c, 500, "Failed to get group statistics")
		return
	}
	c.Header("X-Snapshot-Cache", cacheStatusValue(hit))

	response.Success(c, gin.H{
		"groups":     stats,
		"start_date": startTime.Format("2006-01-02"),
		"end_date":   dashboardResponseEndDate(c, endTime),
	})
}

// GetAPIKeyUsageTrend handles getting API key usage trend data
// GET /api/v1/admin/dashboard/api-keys-trend
// Query params: start_date, end_date (YYYY-MM-DD), granularity (day/hour), limit (default 5)
func (h *DashboardHandler) GetAPIKeyUsageTrend(c *gin.Context) {
	startTime, endTime := parseTimeRange(c)
	granularity := c.DefaultQuery("granularity", "day")
	limitStr := c.DefaultQuery("limit", "5")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 5
	}

	trend, hit, err := h.getAPIKeyUsageTrendCached(c.Request.Context(), startTime, endTime, granularity, limit)
	if err != nil {
		response.Error(c, 500, "Failed to get API key usage trend")
		return
	}
	c.Header("X-Snapshot-Cache", cacheStatusValue(hit))

	response.Success(c, gin.H{
		"trend":       trend,
		"start_date":  startTime.Format("2006-01-02"),
		"end_date":    dashboardResponseEndDate(c, endTime),
		"granularity": granularity,
	})
}

// GetUserUsageTrend handles getting user usage trend data
// GET /api/v1/admin/dashboard/users-trend
// Query params: start_date, end_date (YYYY-MM-DD), granularity (day/hour), limit (default 12)
func (h *DashboardHandler) GetUserUsageTrend(c *gin.Context) {
	startTime, endTime := parseTimeRange(c)
	granularity := c.DefaultQuery("granularity", "day")
	limitStr := c.DefaultQuery("limit", "12")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 12
	}

	trend, hit, err := h.getUserUsageTrendCached(c.Request.Context(), startTime, endTime, granularity, limit)
	if err != nil {
		response.Error(c, 500, "Failed to get user usage trend")
		return
	}
	c.Header("X-Snapshot-Cache", cacheStatusValue(hit))

	response.Success(c, gin.H{
		"trend":       trend,
		"start_date":  startTime.Format("2006-01-02"),
		"end_date":    dashboardResponseEndDate(c, endTime),
		"granularity": granularity,
	})
}

// BatchUsersUsageRequest represents the request body for batch user usage stats
type BatchUsersUsageRequest struct {
	UserIDs []int64 `json:"user_ids" binding:"required"`
}

var dashboardUsersRankingCache = newSnapshotCache(5 * time.Minute)
var dashboardBatchUsersUsageCache = newSnapshotCache(30 * time.Second)
var dashboardBatchAPIKeysUsageCache = newSnapshotCache(30 * time.Second)

func parseRankingLimit(raw string) int {
	limit, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || limit <= 0 {
		return 12
	}
	if limit > 50 {
		return 50
	}
	return limit
}

// GetUserSpendingRanking handles getting user spending ranking data.
// GET /api/v1/admin/dashboard/users-ranking
func (h *DashboardHandler) GetUserSpendingRanking(c *gin.Context) {
	startTime, endTime := parseTimeRange(c)
	responseEndDate := dashboardResponseEndDate(c, endTime)
	limit := parseRankingLimit(c.DefaultQuery("limit", "12"))

	keyRaw, _ := json.Marshal(struct {
		ProjectID       int64  `json:"project_id,omitempty"`
		Start           string `json:"start"`
		End             string `json:"end"`
		ResponseEndDate string `json:"response_end_date"`
		Limit           int    `json:"limit"`
	}{
		ProjectID:       dashboardCacheProjectID(c.Request.Context()),
		Start:           startTime.UTC().Format(time.RFC3339),
		End:             endTime.UTC().Format(time.RFC3339),
		ResponseEndDate: responseEndDate,
		Limit:           limit,
	})
	cacheKey := string(keyRaw)
	if cached, ok := dashboardUsersRankingCache.Get(cacheKey); ok {
		c.Header("X-Snapshot-Cache", "hit")
		response.Success(c, cached.Payload)
		return
	}

	ranking, err := h.dashboardService.GetUserSpendingRanking(c.Request.Context(), startTime, endTime, limit)
	if err != nil {
		response.Error(c, 500, "Failed to get user spending ranking")
		return
	}

	payload := gin.H{
		"ranking":           ranking.Ranking,
		"total_actual_cost": ranking.TotalActualCost,
		"total_requests":    ranking.TotalRequests,
		"total_tokens":      ranking.TotalTokens,
		"start_date":        startTime.Format("2006-01-02"),
		"end_date":          responseEndDate,
	}
	dashboardUsersRankingCache.Set(cacheKey, payload)
	c.Header("X-Snapshot-Cache", "miss")
	response.Success(c, payload)
}

// GetBatchUsersUsage handles getting usage stats for multiple users
// POST /api/v1/admin/dashboard/users-usage
func (h *DashboardHandler) GetBatchUsersUsage(c *gin.Context) {
	var req BatchUsersUsageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	userIDs := normalizeInt64IDList(req.UserIDs)
	if len(userIDs) == 0 {
		response.Success(c, gin.H{"stats": map[string]any{}})
		return
	}

	// cacheKey 必须包含当日日期，否则跨午夜后 30s 内会复用昨天的 "today_*" 结果。
	keyRaw, _ := json.Marshal(struct {
		ProjectID int64   `json:"project_id,omitempty"`
		V         int     `json:"v"`
		Day       string  `json:"day"`
		UserIDs   []int64 `json:"user_ids"`
	}{
		ProjectID: dashboardCacheProjectID(c.Request.Context()),
		V:         2, // bump 当响应结构变化（如加入 by_platform 时）
		Day:       timezone.Today().Format("2006-01-02"),
		UserIDs:   userIDs,
	})
	cacheKey := string(keyRaw)
	if cached, ok := dashboardBatchUsersUsageCache.Get(cacheKey); ok {
		c.Header("X-Snapshot-Cache", "hit")
		response.Success(c, cached.Payload)
		return
	}

	stats, err := h.dashboardService.GetBatchUserUsageStats(c.Request.Context(), userIDs, time.Time{}, time.Time{})
	if err != nil {
		response.Error(c, 500, "Failed to get user usage stats")
		return
	}

	payload := gin.H{"stats": stats}
	dashboardBatchUsersUsageCache.Set(cacheKey, payload)
	c.Header("X-Snapshot-Cache", "miss")
	response.Success(c, payload)
}

// BatchAPIKeysUsageRequest represents the request body for batch api key usage stats
type BatchAPIKeysUsageRequest struct {
	APIKeyIDs []int64 `json:"api_key_ids" binding:"required"`
}

// GetBatchAPIKeysUsage handles getting usage stats for multiple API keys
// POST /api/v1/admin/dashboard/api-keys-usage
func (h *DashboardHandler) GetBatchAPIKeysUsage(c *gin.Context) {
	var req BatchAPIKeysUsageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	apiKeyIDs := normalizeInt64IDList(req.APIKeyIDs)
	if len(apiKeyIDs) == 0 {
		response.Success(c, gin.H{"stats": map[string]any{}})
		return
	}

	keyRaw, _ := json.Marshal(struct {
		ProjectID int64   `json:"project_id,omitempty"`
		APIKeyIDs []int64 `json:"api_key_ids"`
	}{
		ProjectID: dashboardCacheProjectID(c.Request.Context()),
		APIKeyIDs: apiKeyIDs,
	})
	cacheKey := string(keyRaw)
	if cached, ok := dashboardBatchAPIKeysUsageCache.Get(cacheKey); ok {
		c.Header("X-Snapshot-Cache", "hit")
		response.Success(c, cached.Payload)
		return
	}

	stats, err := h.dashboardService.GetBatchAPIKeyUsageStats(c.Request.Context(), apiKeyIDs, time.Time{}, time.Time{})
	if err != nil {
		response.Error(c, 500, "Failed to get API key usage stats")
		return
	}

	payload := gin.H{"stats": stats}
	dashboardBatchAPIKeysUsageCache.Set(cacheKey, payload)
	c.Header("X-Snapshot-Cache", "miss")
	response.Success(c, payload)
}

// GetUserBreakdown handles getting per-user usage breakdown within a dimension.
// GET /api/v1/admin/dashboard/user-breakdown
// Query params: start_date, end_date, group_id, model, endpoint, endpoint_type, limit
func (h *DashboardHandler) GetUserBreakdown(c *gin.Context) {
	startTime, endTime := parseTimeRange(c)

	dim := usagestats.UserBreakdownDimension{}
	if v := c.Query("group_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			dim.GroupID = id
		}
	}
	dim.Model = c.Query("model")
	rawModelSource := strings.TrimSpace(c.DefaultQuery("model_source", usagestats.ModelSourceRequested))
	if !usagestats.IsValidModelSource(rawModelSource) {
		response.BadRequest(c, "Invalid model_source, use requested/upstream/mapping")
		return
	}
	dim.ModelType = rawModelSource
	dim.Endpoint = c.Query("endpoint")
	dim.EndpointType = c.DefaultQuery("endpoint_type", "inbound")

	// Additional filter conditions
	if v := c.Query("user_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			dim.UserID = id
		}
	}
	if v := c.Query("api_key_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			dim.APIKeyID = id
		}
	}
	if v := c.Query("account_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			dim.AccountID = id
		}
	}
	if v := c.Query("request_type"); v != "" {
		if rt, err := strconv.ParseInt(v, 10, 16); err == nil {
			rtVal := int16(rt)
			dim.RequestType = &rtVal
		}
	}
	if v := c.Query("stream"); v != "" {
		if s, err := strconv.ParseBool(v); err == nil {
			dim.Stream = &s
		}
	}
	if v := c.Query("billing_type"); v != "" {
		if bt, err := strconv.ParseInt(v, 10, 8); err == nil {
			btVal := int8(bt)
			dim.BillingType = &btVal
		}
	}

	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	stats, err := h.dashboardService.GetUserBreakdownStats(
		c.Request.Context(), startTime, endTime, dim, limit,
	)
	if err != nil {
		response.Error(c, 500, "Failed to get user breakdown stats")
		return
	}

	response.Success(c, gin.H{
		"users":      stats,
		"start_date": startTime.Format("2006-01-02"),
		"end_date":   dashboardResponseEndDate(c, endTime),
	})
}
