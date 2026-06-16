package admin

import (
	"context"
	"encoding/json"
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
	statsCalls      atomic.Int32
}

func (r *dashboardUsageRepoCacheProbe) GetDashboardStats(ctx context.Context) (*usagestats.DashboardStats, error) {
	r.statsCalls.Add(1)
	return &usagestats.DashboardStats{TotalAccounts: 99, TotalRequests: 123}, nil
}

func (r *dashboardUsageRepoCacheProbe) GetUsageTrendWithFilters(
	ctx context.Context,
	startTime, endTime time.Time,
	granularity string,
	userID, apiKeyID, accountID, groupID int64,
	model string,
	requestType *int16,
	stream *bool,
	billingType *int8,
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

func TestDashboardSnapshotV2OperatorCannotIncludeUsersTrend(t *testing.T) {
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

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int32(0), repo.usersTrendCalls.Load())
	require.NotContains(t, rec.Body.String(), "users_trend")
	require.NotContains(t, rec.Body.String(), "cache@test.dev")
}

func TestDashboardSnapshotV2OperatorEmptyScopeDoesNotReuseAdminCache(t *testing.T) {
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
	require.Equal(t, http.StatusOK, operatorRec.Code)
	require.Equal(t, "miss", operatorRec.Header().Get("X-Snapshot-Cache"))

	var resp struct {
		Data struct {
			Stats struct {
				TotalAccounts int64 `json:"total_accounts"`
			} `json:"stats"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(operatorRec.Body.Bytes(), &resp))
	require.Equal(t, int64(0), resp.Data.Stats.TotalAccounts)
}

func TestDashboardSnapshotV2OperatorStatsUsesScopedAccounts(t *testing.T) {
	t.Cleanup(resetDashboardReadCachesForTest)
	resetDashboardReadCachesForTest()

	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	rateLimitedAt := now.Add(-time.Minute)
	rateLimitResetAt := now.Add(time.Hour)
	overloadUntil := now.Add(time.Hour)
	adminSvc := newStubAdminService()
	adminSvc.accounts = []service.Account{
		{ID: 1, Name: "normal", Status: service.StatusActive, Schedulable: true, GroupIDs: []int64{10}},
		{ID: 2, Name: "error", Status: service.StatusError, GroupIDs: []int64{10}},
		{ID: 3, Name: "rate-limited", Status: service.StatusActive, Schedulable: true, GroupIDs: []int64{20}, RateLimitedAt: &rateLimitedAt, RateLimitResetAt: &rateLimitResetAt},
		{ID: 4, Name: "overload", Status: service.StatusActive, Schedulable: true, GroupIDs: []int64{20}, OverloadUntil: &overloadUntil},
		{ID: 5, Name: "hidden", Status: service.StatusActive, Schedulable: true, GroupIDs: []int64{30}},
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

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data struct {
			Stats struct {
				TotalAccounts     int64 `json:"total_accounts"`
				NormalAccounts    int64 `json:"normal_accounts"`
				ErrorAccounts     int64 `json:"error_accounts"`
				RateLimitAccounts int64 `json:"ratelimit_accounts"`
				OverloadAccounts  int64 `json:"overload_accounts"`
			} `json:"stats"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, int64(4), resp.Data.Stats.TotalAccounts)
	require.Equal(t, int64(3), resp.Data.Stats.NormalAccounts)
	require.Equal(t, int64(1), resp.Data.Stats.ErrorAccounts)
	require.Equal(t, int64(1), resp.Data.Stats.RateLimitAccounts)
	require.Equal(t, int64(1), resp.Data.Stats.OverloadAccounts)
}
