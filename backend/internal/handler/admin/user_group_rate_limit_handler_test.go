package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUserHandler_GetUserGroupRateLimits_ReturnsWindows(t *testing.T) {
	router, _ := setupAdminRouter()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/1/group-rate-limits", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Code int `json:"code"`
		Data struct {
			GroupRateLimits []struct {
				UserID      int64   `json:"user_id"`
				GroupID     int64   `json:"group_id"`
				GroupName   string  `json:"group_name"`
				RateLimit5h float64 `json:"rate_limit_5h"`
				Usage5hUSD  float64 `json:"usage_5h_usd"`
			} `json:"group_rate_limits"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 0, body.Code)
	require.Len(t, body.Data.GroupRateLimits, 1)
	require.Equal(t, int64(1), body.Data.GroupRateLimits[0].UserID)
	require.Equal(t, int64(2), body.Data.GroupRateLimits[0].GroupID)
	require.Equal(t, "group", body.Data.GroupRateLimits[0].GroupName)
	require.Equal(t, 10.0, body.Data.GroupRateLimits[0].RateLimit5h)
	require.Equal(t, 2.5, body.Data.GroupRateLimits[0].Usage5hUSD)
}

func TestUserHandler_ResetUserGroupRateLimit_ReturnsResetWindow(t *testing.T) {
	router, adminSvc := setupAdminRouter()
	windowStart := time.Now().UTC()
	adminSvc.groupRateLimitWindows[0].Usage5hUSD = 4.25
	adminSvc.groupRateLimitWindows[0].Window5hStart = &windowStart

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/1/group-rate-limits/2/reset", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Code int `json:"code"`
		Data struct {
			GroupRateLimit struct {
				UserID          int64   `json:"user_id"`
				GroupID         int64   `json:"group_id"`
				Usage5hUSD      float64 `json:"usage_5h_usd"`
				Window5hStart   *string `json:"window_5h_start"`
				Window5hResetAt *string `json:"window_5h_reset_at"`
			} `json:"group_rate_limit"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 0, body.Code)
	require.Equal(t, int64(1), body.Data.GroupRateLimit.UserID)
	require.Equal(t, int64(2), body.Data.GroupRateLimit.GroupID)
	require.Zero(t, body.Data.GroupRateLimit.Usage5hUSD)
	require.Nil(t, body.Data.GroupRateLimit.Window5hStart)
	require.Nil(t, body.Data.GroupRateLimit.Window5hResetAt)
}

func TestUserHandler_GroupRateLimitRoutesRejectInvalidIDs(t *testing.T) {
	router, _ := setupAdminRouter()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/not-a-user/group-rate-limits", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/1/group-rate-limits/not-a-group/reset", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}
