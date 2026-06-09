package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type adminUsageRepoCapture struct {
	service.UsageLogRepository
	listParams   pagination.PaginationParams
	listFilters  usagestats.UsageLogFilters
	countFilters usagestats.UsageLogFilters
	countTotal   int64
	countErr     error
	statsFilters usagestats.UsageLogFilters
}

func (s *adminUsageRepoCapture) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters usagestats.UsageLogFilters) ([]service.UsageLog, *pagination.PaginationResult, error) {
	s.listParams = params
	s.listFilters = filters
	return []service.UsageLog{}, &pagination.PaginationResult{
		Total:    0,
		Page:     params.Page,
		PageSize: params.PageSize,
		Pages:    0,
	}, nil
}

func (s *adminUsageRepoCapture) GetStatsWithFilters(ctx context.Context, filters usagestats.UsageLogFilters) (*usagestats.UsageStats, error) {
	s.statsFilters = filters
	return &usagestats.UsageStats{}, nil
}

func (s *adminUsageRepoCapture) CountWithFilters(ctx context.Context, filters usagestats.UsageLogFilters) (int64, error) {
	s.countFilters = filters
	if s.countErr != nil {
		return 0, s.countErr
	}
	return s.countTotal, nil
}

func newAdminUsageRequestTypeTestRouter(repo *adminUsageRepoCapture) *gin.Engine {
	gin.SetMode(gin.TestMode)
	usageSvc := service.NewUsageService(repo, nil, nil, nil)
	handler := NewUsageHandler(usageSvc, nil, nil, nil)
	router := gin.New()
	router.GET("/admin/usage", handler.List)
	router.GET("/admin/usage/count", handler.Count)
	router.GET("/admin/usage/stats", handler.Stats)
	return router
}

func TestAdminUsageListRequestTypePriority(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage?request_type=ws_v2&stream=false", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, repo.listFilters.RequestType)
	require.Equal(t, int16(service.RequestTypeWSV2), *repo.listFilters.RequestType)
	require.Nil(t, repo.listFilters.Stream)
}

func TestAdminUsageListInvalidRequestType(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage?request_type=bad", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminUsageListInvalidStream(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage?stream=bad", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminUsageListExactTotalTrue(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage?exact_total=true", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.listFilters.ExactTotal)
}

func TestAdminUsageListInvalidExactTotal(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage?exact_total=oops", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminUsageCountRequestTypePriority(t *testing.T) {
	repo := &adminUsageRepoCapture{countTotal: 42}
	router := newAdminUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage/count?request_type=ws_v2&stream=false", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, repo.countFilters.RequestType)
	require.Equal(t, int16(service.RequestTypeWSV2), *repo.countFilters.RequestType)
	require.Nil(t, repo.countFilters.Stream)
}

func TestAdminUsageListAndCountShareFilterParsing(t *testing.T) {
	repo := &adminUsageRepoCapture{countTotal: 42}
	router := newAdminUsageRequestTypeTestRouter(repo)
	query := "?user_id=11&api_key_id=22&account_id=33&group_id=44&model=gpt-5&billing_type=1&billing_mode=image&request_type=stream&start_date=2026-01-02&end_date=2026-01-03&timezone=UTC"

	listReq := httptest.NewRequest(http.MethodGet, "/admin/usage"+query, nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	require.Equal(t, http.StatusOK, listRec.Code)

	countReq := httptest.NewRequest(http.MethodGet, "/admin/usage/count"+query, nil)
	countRec := httptest.NewRecorder()
	router.ServeHTTP(countRec, countReq)
	require.Equal(t, http.StatusOK, countRec.Code)

	listFilters := repo.listFilters
	countFilters := repo.countFilters
	listFilters.ExactTotal = false
	require.Equal(t, listFilters, countFilters)
}

func TestAdminUsageCountTimeout(t *testing.T) {
	repo := &adminUsageRepoCapture{countErr: context.DeadlineExceeded}
	router := newAdminUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage/count", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusGatewayTimeout, rec.Code)
}

func TestAdminUsageStatsRequestTypePriority(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage/stats?request_type=stream&stream=bad", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, repo.statsFilters.RequestType)
	require.Equal(t, int16(service.RequestTypeStream), *repo.statsFilters.RequestType)
	require.Nil(t, repo.statsFilters.Stream)
}

func TestAdminUsageStatsInvalidRequestType(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage/stats?request_type=oops", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminUsageStatsInvalidStream(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage/stats?stream=oops", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}
