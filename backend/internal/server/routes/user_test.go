package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestUserRoutesRegisterBatchImageAllKeyHistory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")

	RegisterUserRoutes(
		v1,
		&handler.Handlers{BatchImage: &handler.BatchImageHandler{}},
		middleware.JWTAuthMiddleware(func(c *gin.Context) {
			c.AbortWithStatus(http.StatusTeapot)
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/batch-image/jobs", nil)

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusTeapot, rec.Code)
}

func TestUserRoutesBatchImageAllKeyHistoryUsesJWTProjectScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	apiKeyID := int64(22)
	createdAt := time.Unix(1700000000, 0).UTC()
	repo := &batchImageAllKeyHistoryRepo{
		jobs: []*service.BatchImageJob{{
			BatchID:       "imgbatch_user_project",
			UserID:        11,
			ProjectID:     169,
			APIKeyID:      &apiKeyID,
			TaskName:      "demo task",
			Status:        service.BatchImageJobStatusCompleted,
			Provider:      service.BatchImageProviderGeminiAPI,
			Model:         "gemini-3.1-flash-lite-image",
			ItemCount:     1,
			SuccessCount:  1,
			EstimatedCost: 0.2,
			CreatedAt:     createdAt,
		}},
	}

	RegisterUserRoutes(
		v1,
		&handler.Handlers{
			BatchImage: handler.NewBatchImageHandler(&service.BatchImagePublicService{Repo: repo}, nil, nil),
		},
		middleware.JWTAuthMiddleware(func(c *gin.Context) {
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 11})
			c.Request = c.Request.WithContext(service.WithProjectID(c.Request.Context(), 169))
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/user/batch-image/jobs?limit=2&cursor=40&status=completed&task_name=demo&downloaded=false",
		nil,
	)

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(11), repo.userID)
	require.Equal(t, int64(169), repo.projectID)
	require.Equal(t, service.BatchImageJobStatusCompleted, repo.filter.Status)
	require.Equal(t, "demo", repo.filter.TaskNameLike)
	require.NotNil(t, repo.filter.Downloaded)
	require.False(t, *repo.filter.Downloaded)
	require.Equal(t, 3, repo.filter.Limit)
	require.Equal(t, 40, repo.filter.Offset)

	var body struct {
		Object string `json:"object"`
		Data   []struct {
			ID        string  `json:"id"`
			KeyID     int64   `json:"key_id"`
			TaskName  string  `json:"task_name"`
			Status    string  `json:"status"`
			CreatedAt int64   `json:"created_at"`
			Cost      float64 `json:"estimated_cost"`
		} `json:"data"`
		HasMore bool `json:"has_more"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "list", body.Object)
	require.False(t, body.HasMore)
	require.Len(t, body.Data, 1)
	require.Equal(t, "imgbatch_user_project", body.Data[0].ID)
	require.Equal(t, apiKeyID, body.Data[0].KeyID)
	require.Equal(t, "demo task", body.Data[0].TaskName)
	require.Equal(t, service.BatchImageJobStatusCompleted, body.Data[0].Status)
	require.Equal(t, createdAt.Unix(), body.Data[0].CreatedAt)
	require.Equal(t, 0.2, body.Data[0].Cost)
}

func TestBatchImageAllKeyHistoryRequiresAuthAndProject(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name      string
		configure func(*gin.Context)
		wantCode  int
		wantError string
	}{
		{
			name:      "missing auth subject",
			wantCode:  http.StatusUnauthorized,
			wantError: "AUTH_REQUIRED",
		},
		{
			name: "missing project context",
			configure: func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 11})
			},
			wantCode:  http.StatusBadRequest,
			wantError: "PROJECT_REQUIRED",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/user/batch-image/jobs", nil)
			if tc.configure != nil {
				tc.configure(c)
			}

			(&handler.BatchImageHandler{}).ListAll(c)

			require.Equal(t, tc.wantCode, rec.Code)
			require.Contains(t, rec.Body.String(), tc.wantError)
		})
	}
}

type batchImageAllKeyHistoryRepo struct {
	service.BatchImageRepository
	jobs      []*service.BatchImageJob
	userID    int64
	projectID int64
	filter    service.BatchImageJobFilter
}

func (r *batchImageAllKeyHistoryRepo) ListBatchImageJobsForUser(ctx context.Context, userID int64, filter service.BatchImageJobFilter) ([]*service.BatchImageJob, error) {
	r.userID = userID
	r.projectID, _ = service.ProjectIDFromContext(ctx)
	r.filter = filter
	return r.jobs, nil
}
