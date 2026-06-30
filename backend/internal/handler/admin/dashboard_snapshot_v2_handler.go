package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

var dashboardSnapshotV2Cache = newSnapshotCache(30 * time.Second)

type dashboardSnapshotV2Stats struct {
	usagestats.DashboardStats
	Uptime int64 `json:"uptime"`
}

type dashboardSnapshotV2Response struct {
	GeneratedAt string `json:"generated_at"`

	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	Granularity string `json:"granularity"`

	Stats      *dashboardSnapshotV2Stats        `json:"stats,omitempty"`
	Trend      []usagestats.TrendDataPoint      `json:"trend,omitempty"`
	Models     []usagestats.ModelStat           `json:"models,omitempty"`
	Groups     []usagestats.GroupStat           `json:"groups,omitempty"`
	UsersTrend []usagestats.UserUsageTrendPoint `json:"users_trend,omitempty"`
}

type dashboardSnapshotV2Filters struct {
	UserID            int64
	APIKeyID          int64
	AccountID         int64
	GroupID           int64
	Model             string
	ModelSource       string
	ModelFilterSource string
	RequestType       *int16
	Stream            *bool
	BillingType       *int8
	BillingMode       string
}

func (f *dashboardSnapshotV2Filters) usageLogFilters() usagestats.UsageLogFilters {
	if f == nil {
		return usagestats.UsageLogFilters{}
	}
	return usagestats.UsageLogFilters{
		UserID:            f.UserID,
		APIKeyID:          f.APIKeyID,
		AccountID:         f.AccountID,
		GroupID:           f.GroupID,
		Model:             f.Model,
		ModelFilterSource: f.ModelFilterSource,
		RequestType:       f.RequestType,
		Stream:            f.Stream,
		BillingType:       f.BillingType,
		BillingMode:       f.BillingMode,
	}
}

type dashboardSnapshotV2CacheKey struct {
	ProjectID         int64   `json:"project_id,omitempty"`
	StartTime         string  `json:"start_time"`
	EndTime           string  `json:"end_time"`
	ResponseEndDate   string  `json:"response_end_date"`
	Granularity       string  `json:"granularity"`
	Scoped            bool    `json:"scoped"`
	ScopeEmpty        bool    `json:"scope_empty"`
	ScopeGroupIDs     []int64 `json:"scope_group_ids"`
	UserID            int64   `json:"user_id"`
	APIKeyID          int64   `json:"api_key_id"`
	AccountID         int64   `json:"account_id"`
	GroupID           int64   `json:"group_id"`
	Model             string  `json:"model"`
	ModelFilterSource string  `json:"model_filter_source,omitempty"`
	ModelSource       string  `json:"model_source,omitempty"`
	RequestType       *int16  `json:"request_type"`
	Stream            *bool   `json:"stream"`
	BillingType       *int8   `json:"billing_type"`
	BillingMode       string  `json:"billing_mode,omitempty"`
	IncludeStats      bool    `json:"include_stats"`
	IncludeTrend      bool    `json:"include_trend"`
	IncludeModels     bool    `json:"include_models"`
	IncludeGroups     bool    `json:"include_groups"`
	IncludeUsersTrend bool    `json:"include_users_trend"`
	UsersTrendLimit   int     `json:"users_trend_limit"`
}

func (h *DashboardHandler) GetSnapshotV2(c *gin.Context) {
	scope, scopeErr := resolveAdminAccessScope(c, h.permissionService)
	if scopeErr != nil {
		response.ErrorFrom(c, scopeErr)
		return
	}
	startTime, endTime := parseTimeRange(c)
	granularity := strings.TrimSpace(c.DefaultQuery("granularity", "day"))
	if granularity != "hour" {
		granularity = "day"
	}

	includeStats := parseBoolQueryWithDefault(c.Query("include_stats"), true)
	includeTrend := parseBoolQueryWithDefault(c.Query("include_trend"), true)
	includeModels := parseBoolQueryWithDefault(c.Query("include_model_stats"), true)
	includeGroups := parseBoolQueryWithDefault(c.Query("include_group_stats"), false)
	includeUsersTrend := parseBoolQueryWithDefault(c.Query("include_users_trend"), false)
	usersTrendLimit := 12
	if raw := strings.TrimSpace(c.Query("users_trend_limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 50 {
			usersTrendLimit = parsed
		}
	}

	filters, err := parseDashboardSnapshotV2Filters(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	responseEndDate := dashboardResponseEndDate(c, endTime)
	scopeGroupIDs, err := scope.dashboardGroupIDs(filters.GroupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if scope.isScoped() {
		includeUsersTrend = false
	}

	keyRaw, _ := json.Marshal(dashboardSnapshotV2CacheKey{
		ProjectID:         dashboardCacheProjectID(c.Request.Context()),
		StartTime:         startTime.UTC().Format(time.RFC3339),
		EndTime:           endTime.UTC().Format(time.RFC3339),
		ResponseEndDate:   responseEndDate,
		Granularity:       granularity,
		Scoped:            scope.isScoped(),
		ScopeEmpty:        scope.isScoped() && len(scopeGroupIDs) == 0,
		ScopeGroupIDs:     scopeGroupIDs,
		UserID:            filters.UserID,
		APIKeyID:          filters.APIKeyID,
		AccountID:         filters.AccountID,
		GroupID:           filters.GroupID,
		Model:             filters.Model,
		ModelFilterSource: usagestats.NormalizeModelSource(filters.ModelFilterSource),
		ModelSource:       usagestats.NormalizeModelSource(filters.ModelSource),
		RequestType:       filters.RequestType,
		Stream:            filters.Stream,
		BillingType:       filters.BillingType,
		BillingMode:       filters.BillingMode,
		IncludeStats:      includeStats,
		IncludeTrend:      includeTrend,
		IncludeModels:     includeModels,
		IncludeGroups:     includeGroups,
		IncludeUsersTrend: includeUsersTrend,
		UsersTrendLimit:   usersTrendLimit,
	})
	cacheKey := string(keyRaw)

	cached, hit, err := dashboardSnapshotV2Cache.GetOrLoad(cacheKey, func() (any, error) {
		return h.buildSnapshotV2Response(
			c.Request.Context(),
			startTime,
			endTime,
			responseEndDate,
			granularity,
			scope,
			filters,
			includeStats,
			includeTrend,
			includeModels,
			includeGroups,
			includeUsersTrend,
			usersTrendLimit,
		)
	})
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	if cached.ETag != "" {
		c.Header("ETag", cached.ETag)
		c.Header("Vary", "If-None-Match")
		if ifNoneMatchMatched(c.GetHeader("If-None-Match"), cached.ETag) {
			c.Status(http.StatusNotModified)
			return
		}
	}
	c.Header("X-Snapshot-Cache", cacheStatusValue(hit))
	response.Success(c, cached.Payload)
}

func (h *DashboardHandler) buildSnapshotV2Response(
	ctx context.Context,
	startTime, endTime time.Time,
	responseEndDate string,
	granularity string,
	scope *adminAccessScope,
	filters *dashboardSnapshotV2Filters,
	includeStats, includeTrend, includeModels, includeGroups, includeUsersTrend bool,
	usersTrendLimit int,
) (*dashboardSnapshotV2Response, error) {
	resp := &dashboardSnapshotV2Response{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		StartDate:   startTime.Format("2006-01-02"),
		EndDate:     responseEndDate,
		Granularity: granularity,
	}

	if includeStats {
		var stats *usagestats.DashboardStats
		var err error
		if scope != nil && scope.isScoped() {
			stats, err = h.getDashboardStatsForScope(ctx, scope, startTime, endTime)
		} else {
			stats, err = h.dashboardService.GetDashboardStats(ctx)
		}
		if err != nil {
			return nil, errors.New("failed to get dashboard statistics")
		}
		resp.Stats = &dashboardSnapshotV2Stats{
			DashboardStats: *stats,
			Uptime:         int64(time.Since(h.startTime).Seconds()),
		}
	}

	if includeTrend {
		trend, _, err := h.getUsageTrendForScope(ctx, scope, startTime, endTime, granularity, filters.usageLogFilters())
		if err != nil {
			return nil, errors.New("failed to get usage trend")
		}
		resp.Trend = trend
	}

	if includeModels {
		models, _, err := h.getModelStatsForScope(ctx, scope, startTime, endTime, filters.usageLogFilters(), filters.ModelSource)
		if err != nil {
			return nil, errors.New("failed to get model statistics")
		}
		resp.Models = models
	}

	if includeGroups {
		groups, _, err := h.getGroupStatsForScope(ctx, scope, startTime, endTime, filters.usageLogFilters())
		if err != nil {
			return nil, errors.New("failed to get group statistics")
		}
		resp.Groups = groups
	}

	if includeUsersTrend {
		usersTrend, _, err := h.getUserUsageTrendCached(ctx, startTime, endTime, granularity, usersTrendLimit)
		if err != nil {
			return nil, errors.New("failed to get user usage trend")
		}
		resp.UsersTrend = usersTrend
	}

	return resp, nil
}

func parseDashboardSnapshotV2Filters(c *gin.Context) (*dashboardSnapshotV2Filters, error) {
	filters := &dashboardSnapshotV2Filters{
		Model:             strings.TrimSpace(c.Query("model")),
		ModelSource:       usagestats.ModelSourceRequested,
		ModelFilterSource: usagestats.ModelSourceRequested,
		BillingMode:       strings.TrimSpace(c.Query("billing_mode")),
	}
	if rawModelFilterSource := strings.TrimSpace(c.Query("model_filter_source")); rawModelFilterSource != "" {
		if !usagestats.IsValidModelSource(rawModelFilterSource) {
			return nil, errors.New("invalid model_filter_source, use requested/upstream/mapping")
		}
		filters.ModelFilterSource = rawModelFilterSource
	}
	if rawModelSource := strings.TrimSpace(c.Query("model_source")); rawModelSource != "" {
		if !usagestats.IsValidModelSource(rawModelSource) {
			return nil, errors.New("invalid model_source, use requested/upstream/mapping")
		}
		filters.ModelSource = rawModelSource
	}
	if filters.BillingMode != "" && !service.BillingMode(filters.BillingMode).IsValid() {
		return nil, errors.New("invalid billing_mode")
	}

	if userIDStr := strings.TrimSpace(c.Query("user_id")); userIDStr != "" {
		id, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil {
			return nil, err
		}
		filters.UserID = id
	}
	if apiKeyIDStr := strings.TrimSpace(c.Query("api_key_id")); apiKeyIDStr != "" {
		id, err := strconv.ParseInt(apiKeyIDStr, 10, 64)
		if err != nil {
			return nil, err
		}
		filters.APIKeyID = id
	}
	if accountIDStr := strings.TrimSpace(c.Query("account_id")); accountIDStr != "" {
		id, err := strconv.ParseInt(accountIDStr, 10, 64)
		if err != nil {
			return nil, err
		}
		filters.AccountID = id
	}
	if groupIDStr := strings.TrimSpace(c.Query("group_id")); groupIDStr != "" {
		id, err := strconv.ParseInt(groupIDStr, 10, 64)
		if err != nil {
			return nil, err
		}
		filters.GroupID = id
	}

	if requestTypeStr := strings.TrimSpace(c.Query("request_type")); requestTypeStr != "" {
		parsed, err := service.ParseUsageRequestType(requestTypeStr)
		if err != nil {
			return nil, err
		}
		value := int16(parsed)
		filters.RequestType = &value
	} else if streamStr := strings.TrimSpace(c.Query("stream")); streamStr != "" {
		streamVal, err := strconv.ParseBool(streamStr)
		if err != nil {
			return nil, err
		}
		filters.Stream = &streamVal
	}

	if billingTypeStr := strings.TrimSpace(c.Query("billing_type")); billingTypeStr != "" {
		v, err := strconv.ParseInt(billingTypeStr, 10, 8)
		if err != nil {
			return nil, err
		}
		bt := int8(v)
		filters.BillingType = &bt
	}

	return filters, nil
}
