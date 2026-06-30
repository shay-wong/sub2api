package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type dashboardUsageRepoCacheProbe struct {
	service.UsageLogRepository
	trendCalls      atomic.Int32
	usersTrendCalls atomic.Int32
	modelCalls      atomic.Int32
	modelFilters    []usagestats.UsageLogFilters
	groupCalls      atomic.Int32
	groupFilters    []usagestats.UsageLogFilters
	statsCalls      atomic.Int32
	statsProjectIDs []int64
	ranking         []usagestats.UserSpendingRankingItem
	rankingTotal    float64
}

func (r *dashboardUsageRepoCacheProbe) GetDashboardStats(ctx context.Context) (*usagestats.DashboardStats, error) {
	r.statsCalls.Add(1)
	if projectID, ok := service.ProjectIDFromContext(ctx); ok {
		r.statsProjectIDs = append(r.statsProjectIDs, projectID)
		return &usagestats.DashboardStats{TotalAccounts: 4, TotalRequests: 12}, nil
	}
	return &usagestats.DashboardStats{TotalAccounts: 99, TotalRequests: 123}, nil
}

func (r *dashboardUsageRepoCacheProbe) GetUsageTrendWithUsageFilters(
	ctx context.Context,
	startTime, endTime time.Time,
	granularity string,
	filters usagestats.UsageLogFilters,
) ([]usagestats.TrendDataPoint, error) {
	r.trendCalls.Add(1)
	return []usagestats.TrendDataPoint{{
		Date:        "2026-03-11",
		Requests:    1,
		TotalTokens: 2,
		Cost:        3,
		ActualCost:  4,
	}}, nil
}

func (r *dashboardUsageRepoCacheProbe) GetModelStatsWithUsageFiltersBySource(
	ctx context.Context,
	startTime, endTime time.Time,
	filters usagestats.UsageLogFilters,
	source string,
) ([]usagestats.ModelStat, error) {
	r.modelCalls.Add(1)
	r.modelFilters = append(r.modelFilters, filters)
	return []usagestats.ModelStat{{
		Model:    filters.Model,
		Requests: 1,
	}}, nil
}

func (r *dashboardUsageRepoCacheProbe) GetGroupStatsWithUsageFilters(
	ctx context.Context,
	startTime, endTime time.Time,
	filters usagestats.UsageLogFilters,
) ([]usagestats.GroupStat, error) {
	r.groupCalls.Add(1)
	r.groupFilters = append(r.groupFilters, filters)
	return []usagestats.GroupStat{{
		GroupID:  1,
		Requests: 1,
	}}, nil
}

func (r *dashboardUsageRepoCacheProbe) GetUserUsageTrend(
	ctx context.Context,
	startTime, endTime time.Time,
	granularity string,
	limit int,
) ([]usagestats.UserUsageTrendPoint, error) {
	r.usersTrendCalls.Add(1)
	return []usagestats.UserUsageTrendPoint{{
		Date:       "2026-03-11",
		UserID:     1,
		Email:      "cache@test.dev",
		Requests:   2,
		Tokens:     20,
		Cost:       2,
		ActualCost: 1,
	}}, nil
}

func (r *dashboardUsageRepoCacheProbe) GetUserSpendingRanking(
	ctx context.Context,
	startTime, endTime time.Time,
	limit int,
) (*usagestats.UserSpendingRankingResponse, error) {
	return &usagestats.UserSpendingRankingResponse{
		Ranking:         r.ranking,
		TotalActualCost: r.rankingTotal,
		TotalRequests:   44,
		TotalTokens:     1234,
	}, nil
}

func resetDashboardReadCachesForTest() {
	dashboardTrendCache = newSnapshotCache(30 * time.Second)
	dashboardUsersTrendCache = newSnapshotCache(30 * time.Second)
	dashboardAPIKeysTrendCache = newSnapshotCache(30 * time.Second)
	dashboardModelStatsCache = newSnapshotCache(30 * time.Second)
	dashboardGroupStatsCache = newSnapshotCache(30 * time.Second)
	dashboardSnapshotV2Cache = newSnapshotCache(30 * time.Second)
}

func TestDashboardHandler_GetUsageTrend_UsesCache(t *testing.T) {
	t.Cleanup(resetDashboardReadCachesForTest)
	resetDashboardReadCachesForTest()

	gin.SetMode(gin.TestMode)
	repo := &dashboardUsageRepoCacheProbe{}
	dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
	handler := NewDashboardHandler(dashboardSvc, nil)
	router := gin.New()
	router.GET("/admin/dashboard/trend", handler.GetUsageTrend)

	req1 := httptest.NewRequest(http.MethodGet, "/admin/dashboard/trend?start_date=2026-03-01&end_date=2026-03-07&granularity=day", nil)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	require.Equal(t, http.StatusOK, rec1.Code)
	require.Equal(t, "miss", rec1.Header().Get("X-Snapshot-Cache"))

	req2 := httptest.NewRequest(http.MethodGet, "/admin/dashboard/trend?start_date=2026-03-01&end_date=2026-03-07&granularity=day", nil)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code)
	require.Equal(t, "hit", rec2.Header().Get("X-Snapshot-Cache"))
	require.Equal(t, int32(1), repo.trendCalls.Load())
}

func TestDashboardHandler_GetUserUsageTrend_UsesCache(t *testing.T) {
	t.Cleanup(resetDashboardReadCachesForTest)
	resetDashboardReadCachesForTest()

	gin.SetMode(gin.TestMode)
	repo := &dashboardUsageRepoCacheProbe{}
	dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
	handler := NewDashboardHandler(dashboardSvc, nil)
	router := gin.New()
	router.GET("/admin/dashboard/users-trend", handler.GetUserUsageTrend)

	req1 := httptest.NewRequest(http.MethodGet, "/admin/dashboard/users-trend?start_date=2026-03-01&end_date=2026-03-07&granularity=day&limit=8", nil)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	require.Equal(t, http.StatusOK, rec1.Code)
	require.Equal(t, "miss", rec1.Header().Get("X-Snapshot-Cache"))

	req2 := httptest.NewRequest(http.MethodGet, "/admin/dashboard/users-trend?start_date=2026-03-01&end_date=2026-03-07&granularity=day&limit=8", nil)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code)
	require.Equal(t, "hit", rec2.Header().Get("X-Snapshot-Cache"))
	require.Equal(t, int32(1), repo.usersTrendCalls.Load())
}

func TestDashboardHandler_GetStats_ProjectScopedDoesNotReuseGlobalStats(t *testing.T) {
	t.Cleanup(resetDashboardReadCachesForTest)
	resetDashboardReadCachesForTest()

	gin.SetMode(gin.TestMode)
	repo := &dashboardUsageRepoCacheProbe{}
	dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
	handler := NewDashboardHandler(dashboardSvc, nil)
	router := gin.New()
	router.GET("/admin/dashboard/stats", handler.GetStats)

	globalReq := httptest.NewRequest(http.MethodGet, "/admin/dashboard/stats", nil)
	globalRec := httptest.NewRecorder()
	router.ServeHTTP(globalRec, globalReq)
	require.Equal(t, http.StatusOK, globalRec.Code)
	require.Contains(t, globalRec.Body.String(), "\"total_accounts\":99")

	projectReq := httptest.NewRequest(http.MethodGet, "/admin/dashboard/stats", nil)
	projectReq = projectReq.WithContext(service.WithProjectID(projectReq.Context(), 42))
	projectRec := httptest.NewRecorder()
	router.ServeHTTP(projectRec, projectReq)
	require.Equal(t, http.StatusOK, projectRec.Code)
	require.Contains(t, projectRec.Body.String(), "\"total_accounts\":4")
	require.Equal(t, []int64{42}, repo.statsProjectIDs)
}

func TestDashboardSnapshotV2_ProjectScopedStatsDoesNotReuseGlobalCache(t *testing.T) {
	t.Cleanup(resetDashboardReadCachesForTest)
	resetDashboardReadCachesForTest()

	gin.SetMode(gin.TestMode)
	repo := &dashboardUsageRepoCacheProbe{}
	dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
	handler := NewDashboardHandler(dashboardSvc, nil)
	router := gin.New()
	router.GET("/admin/dashboard/snapshot-v2", handler.GetSnapshotV2)

	path := "/admin/dashboard/snapshot-v2?start_date=2026-03-01&end_date=2026-03-07&include_trend=false&include_model_stats=false&include_group_stats=false"
	globalReq := httptest.NewRequest(http.MethodGet, path, nil)
	globalRec := httptest.NewRecorder()
	router.ServeHTTP(globalRec, globalReq)
	require.Equal(t, http.StatusOK, globalRec.Code)
	require.Equal(t, "miss", globalRec.Header().Get("X-Snapshot-Cache"))
	require.Contains(t, globalRec.Body.String(), "\"total_accounts\":99")

	projectReq := httptest.NewRequest(http.MethodGet, path, nil)
	projectReq = projectReq.WithContext(service.WithProjectID(projectReq.Context(), 42))
	projectRec := httptest.NewRecorder()
	router.ServeHTTP(projectRec, projectReq)
	require.Equal(t, http.StatusOK, projectRec.Code)
	require.Equal(t, "miss", projectRec.Header().Get("X-Snapshot-Cache"))
	require.Contains(t, projectRec.Body.String(), "\"total_accounts\":4")
	require.Equal(t, []int64{42}, repo.statsProjectIDs)
}

func TestDashboardSnapshotV2RejectsLegacyOperatorRole(t *testing.T) {
	t.Cleanup(resetDashboardReadCachesForTest)
	resetDashboardReadCachesForTest()

	gin.SetMode(gin.TestMode)
	repo := &dashboardUsageRepoCacheProbe{}
	dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
	permissionSvc := service.NewPermissionService(
		&operatorPermissionRepoStub{scopes: map[int64][]int64{101: []int64{10}}},
		operatorUserRepoStub{},
		operatorGroupRepoStub{},
	)
	handler := NewDashboardHandler(dashboardSvc, nil, permissionSvc)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 101})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleOperator)
		c.Next()
	})
	router.GET("/admin/dashboard/snapshot-v2", handler.GetSnapshotV2)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/snapshot-v2?start_date=2026-03-01&end_date=2026-03-07&include_stats=false&include_trend=false&include_model_stats=false&include_group_stats=false&include_users_trend=true", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "LEGACY_OPERATOR_ROLE_DISABLED")
	require.Equal(t, int32(0), repo.usersTrendCalls.Load())
	require.NotContains(t, rec.Body.String(), "cache@test.dev")
}

func TestDashboardSnapshotV2LegacyOperatorDoesNotReuseAdminCache(t *testing.T) {
	t.Cleanup(resetDashboardReadCachesForTest)
	resetDashboardReadCachesForTest()

	gin.SetMode(gin.TestMode)
	repo := &dashboardUsageRepoCacheProbe{}
	dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
	permissionSvc := service.NewPermissionService(
		&operatorPermissionRepoStub{scopes: map[int64][]int64{101: []int64{}}},
		operatorUserRepoStub{},
		operatorGroupRepoStub{},
	)
	handler := NewDashboardHandler(dashboardSvc, nil, permissionSvc)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		role := c.GetHeader("X-Test-Role")
		if role == "" {
			role = service.RoleAdmin
		}
		userID := int64(1)
		if role == service.RoleOperator {
			userID = 101
		}
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: userID})
		c.Set(string(middleware.ContextKeyUserRole), role)
		c.Next()
	})
	router.GET("/admin/dashboard/snapshot-v2", handler.GetSnapshotV2)

	path := "/admin/dashboard/snapshot-v2?start_date=2026-03-01&end_date=2026-03-07&include_trend=false&include_model_stats=false&include_group_stats=false"
	adminReq := httptest.NewRequest(http.MethodGet, path, nil)
	adminRec := httptest.NewRecorder()
	router.ServeHTTP(adminRec, adminReq)
	require.Equal(t, http.StatusOK, adminRec.Code)
	require.Equal(t, "miss", adminRec.Header().Get("X-Snapshot-Cache"))
	require.Contains(t, adminRec.Body.String(), "\"total_accounts\":99")

	operatorReq := httptest.NewRequest(http.MethodGet, path, nil)
	operatorReq.Header.Set("X-Test-Role", service.RoleOperator)
	operatorRec := httptest.NewRecorder()
	router.ServeHTTP(operatorRec, operatorReq)
	require.Equal(t, http.StatusForbidden, operatorRec.Code)
	require.Empty(t, operatorRec.Header().Get("X-Snapshot-Cache"))
	require.Contains(t, operatorRec.Body.String(), "LEGACY_OPERATOR_ROLE_DISABLED")
}

func TestDashboardSnapshotV2LegacyOperatorStatsRejectedBeforeAccountScope(t *testing.T) {
	t.Cleanup(resetDashboardReadCachesForTest)
	resetDashboardReadCachesForTest()

	gin.SetMode(gin.TestMode)
	adminSvc := newStubAdminService()
	adminSvc.accounts = []service.Account{
		{ID: 1, Name: "normal", Status: service.StatusActive, Schedulable: true, GroupIDs: []int64{10}},
	}
	repo := &dashboardUsageRepoCacheProbe{}
	dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
	permissionSvc := service.NewPermissionService(
		&operatorPermissionRepoStub{scopes: map[int64][]int64{101: []int64{10, 20}}},
		operatorUserRepoStub{},
		operatorGroupRepoStub{},
	)
	handler := NewDashboardHandler(dashboardSvc, nil, permissionSvc)
	handler.SetAdminService(adminSvc)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 101})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleOperator)
		c.Next()
	})
	router.GET("/admin/dashboard/snapshot-v2", handler.GetSnapshotV2)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/snapshot-v2?start_date=2026-03-01&end_date=2026-03-07&include_trend=false&include_model_stats=false&include_group_stats=false", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "LEGACY_OPERATOR_ROLE_DISABLED")
}

func TestDashboardSnapshotV2ModelFilterSeparatesCacheEntries(t *testing.T) {
	t.Cleanup(resetDashboardReadCachesForTest)
	resetDashboardReadCachesForTest()

	gin.SetMode(gin.TestMode)
	repo := &dashboardUsageRepoCacheProbe{}
	dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
	handler := NewDashboardHandler(dashboardSvc, nil)
	router := gin.New()
	router.GET("/admin/dashboard/snapshot-v2", handler.GetSnapshotV2)

	base := "/admin/dashboard/snapshot-v2?start_date=2026-03-01&end_date=2026-03-07&include_stats=false&include_trend=false&include_model_stats=true&include_group_stats=true&billing_mode=token&"
	req1 := httptest.NewRequest(http.MethodGet, base+"model=gpt-5", nil)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	require.Equal(t, http.StatusOK, rec1.Code)
	require.Equal(t, "miss", rec1.Header().Get("X-Snapshot-Cache"))

	req2 := httptest.NewRequest(http.MethodGet, base+"model=claude-sonnet-4-5", nil)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code)
	require.Equal(t, "miss", rec2.Header().Get("X-Snapshot-Cache"))

	require.Equal(t, int32(2), repo.modelCalls.Load())
	require.Equal(t, int32(2), repo.groupCalls.Load())
	require.Len(t, repo.modelFilters, 2)
	require.Len(t, repo.groupFilters, 2)
	require.Equal(t, "gpt-5", repo.modelFilters[0].Model)
	require.Equal(t, "claude-sonnet-4-5", repo.modelFilters[1].Model)
	require.Equal(t, "token", repo.modelFilters[0].BillingMode)
	require.Equal(t, "token", repo.groupFilters[1].BillingMode)
}

func TestDashboardSnapshotV2ResponseEndDateSeparatesCacheEntries(t *testing.T) {
	t.Cleanup(resetDashboardReadCachesForTest)
	resetDashboardReadCachesForTest()

	gin.SetMode(gin.TestMode)
	repo := &dashboardUsageRepoCacheProbe{}
	dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
	handler := NewDashboardHandler(dashboardSvc, nil)
	router := gin.New()
	router.GET("/admin/dashboard/snapshot-v2", handler.GetSnapshotV2)

	base := "/admin/dashboard/snapshot-v2?include_stats=true&include_trend=false&include_model_stats=false&include_group_stats=false"
	req1 := httptest.NewRequest(http.MethodGet, base+"&start_date=2026-03-01&end_date=2026-03-07", nil)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	require.Equal(t, http.StatusOK, rec1.Code)
	require.Equal(t, "miss", rec1.Header().Get("X-Snapshot-Cache"))
	require.Contains(t, rec1.Body.String(), `"end_date":"2026-03-07"`)

	req2 := httptest.NewRequest(http.MethodGet, base+"&start_date=2026-03-01&end_date=bad", nil)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code)
	require.Equal(t, "miss", rec2.Header().Get("X-Snapshot-Cache"))
	require.NotContains(t, rec2.Body.String(), `"end_date":"bad"`)
	require.NotContains(t, rec2.Body.String(), `"end_date":"2026-03-07"`)

	req3 := httptest.NewRequest(http.MethodGet, base+"&start_date=2026-03-01&end_date=2026-03-07", nil)
	rec3 := httptest.NewRecorder()
	router.ServeHTTP(rec3, req3)
	require.Equal(t, http.StatusOK, rec3.Code)
	require.Equal(t, "hit", rec3.Header().Get("X-Snapshot-Cache"))
	require.Contains(t, rec3.Body.String(), `"end_date":"2026-03-07"`)
}

func TestDashboardUsersRankingResponseEndDateSeparatesCacheEntries(t *testing.T) {
	t.Cleanup(resetDashboardReadCachesForTest)
	resetDashboardReadCachesForTest()

	gin.SetMode(gin.TestMode)
	repo := &dashboardUsageRepoCacheProbe{
		ranking: []usagestats.UserSpendingRankingItem{
			{UserID: 7, Email: "rank@example.com", ActualCost: 10.5, Requests: 3, Tokens: 300},
		},
		rankingTotal: 88.8,
	}
	dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
	handler := NewDashboardHandler(dashboardSvc, nil)
	router := gin.New()
	router.GET("/admin/dashboard/users-ranking", handler.GetUserSpendingRanking)

	req1 := httptest.NewRequest(http.MethodGet, "/admin/dashboard/users-ranking?start_date=2026-03-01&end_date=2026-03-07", nil)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	require.Equal(t, http.StatusOK, rec1.Code)
	require.Equal(t, "miss", rec1.Header().Get("X-Snapshot-Cache"))
	require.Contains(t, rec1.Body.String(), `"end_date":"2026-03-07"`)

	req2 := httptest.NewRequest(http.MethodGet, "/admin/dashboard/users-ranking?start_date=2026-03-01&end_date=bad", nil)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code)
	require.Equal(t, "miss", rec2.Header().Get("X-Snapshot-Cache"))
	require.NotContains(t, rec2.Body.String(), `"end_date":"bad"`)
	require.NotContains(t, rec2.Body.String(), `"end_date":"2026-03-07"`)

	req3 := httptest.NewRequest(http.MethodGet, "/admin/dashboard/users-ranking?start_date=2026-03-01&end_date=2026-03-07", nil)
	rec3 := httptest.NewRecorder()
	router.ServeHTTP(rec3, req3)
	require.Equal(t, http.StatusOK, rec3.Code)
	require.Equal(t, "hit", rec3.Header().Get("X-Snapshot-Cache"))
	require.Contains(t, rec3.Body.String(), `"end_date":"2026-03-07"`)
}
