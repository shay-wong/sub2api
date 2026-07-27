package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type routePanelRateLimitSettingRepo struct {
	service.SettingRepository
	value string
}

func (r *routePanelRateLimitSettingRepo) GetValue(context.Context, string) (string, error) {
	return r.value, nil
}

func newRoutePanelRateLimiter(t *testing.T, settings string) *servermiddleware.PanelRateLimiter {
	t.Helper()
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { require.NoError(t, redisClient.Close()) })
	settingService := service.NewSettingService(&routePanelRateLimitSettingRepo{value: settings}, &config.Config{})
	return servermiddleware.NewPanelRateLimiter(redisClient, settingService)
}

func routePanelAdminAuth(c *gin.Context) {
	c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 42})
	c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleSuperAdmin)
	c.Next()
}

func performRoutePanelRequests(t *testing.T, router *gin.Engine, method, path string, allowed int) {
	t.Helper()
	for i := 0; i < allowed; i++ {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(method, path, nil)
		router.ServeHTTP(recorder, request)
		require.NotEqual(t, http.StatusTooManyRequests, recorder.Code, "request %d should pass the limiter", i+1)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, nil)
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.NotEmpty(t, recorder.Header().Get("Retry-After"))
}

func TestAdminPaymentRoutesApplyGlobalPanelRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery())
	limiter := newRoutePanelRateLimiter(t, `{"enabled":true,"user_rpm":1,"heavy_rpm":10,"exempt_admin":false,"public_ip_rpm":0}`)

	RegisterPaymentRoutes(
		router.Group("/api/v1"),
		&handler.PaymentHandler{},
		&handler.PaymentWebhookHandler{},
		&adminhandler.PaymentHandler{},
		servermiddleware.JWTAuthMiddleware(routePanelAdminAuth),
		servermiddleware.AdminAuthMiddleware(routePanelAdminAuth),
		servermiddleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() }),
		nil,
		limiter,
	)

	performRoutePanelRequests(t, router, http.MethodGet, "/api/v1/admin/payment/config", 1)
}

func TestAggregateAdminRoutesApplyHeavyPanelRateLimit(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		payment bool
	}{
		{name: "admin dashboard", path: "/api/v1/admin/dashboard/stats"},
		{name: "admin usage", path: "/api/v1/admin/usage/stats"},
		{name: "ops dashboard", path: "/api/v1/admin/ops/dashboard/overview"},
		{name: "payment dashboard", path: "/api/v1/admin/payment/dashboard", payment: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(gin.Recovery())
			limiter := newRoutePanelRateLimiter(t, `{"enabled":true,"user_rpm":10,"heavy_rpm":1,"exempt_admin":false,"public_ip_rpm":0}`)

			if tt.payment {
				RegisterPaymentRoutes(
					router.Group("/api/v1"),
					&handler.PaymentHandler{},
					&handler.PaymentWebhookHandler{},
					&adminhandler.PaymentHandler{},
					servermiddleware.JWTAuthMiddleware(routePanelAdminAuth),
					servermiddleware.AdminAuthMiddleware(routePanelAdminAuth),
					servermiddleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() }),
					nil,
					limiter,
				)
			} else {
				handlers := &handler.Handlers{Admin: &handler.AdminHandlers{
					Dashboard: &adminhandler.DashboardHandler{},
					Ops:       adminhandler.NewOpsHandler(nil),
					Usage:     adminhandler.NewUsageHandler(nil, nil, nil, nil),
				}}
				RegisterAdminRoutes(
					router.Group("/api/v1"),
					handlers,
					servermiddleware.AdminAuthMiddleware(routePanelAdminAuth),
					servermiddleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() }),
					servermiddleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() }),
					nil,
					limiter,
				)
			}

			performRoutePanelRequests(t, router, http.MethodGet, tt.path, 1)
		})
	}
}
