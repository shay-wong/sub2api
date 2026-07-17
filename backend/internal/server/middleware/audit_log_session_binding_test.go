//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type sessionBindingAuditRepo struct {
	mu     sync.Mutex
	nextID int64
	logs   []*service.AuditLog
}

func (r *sessionBindingAuditRepo) NextID(context.Context) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	return r.nextID, nil
}

func (r *sessionBindingAuditRepo) BatchInsert(_ context.Context, logs []*service.AuditLog) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, logs...)
	return int64(len(logs)), nil
}

func (*sessionBindingAuditRepo) ClearAll(context.Context, *service.AuditLog) (int64, error) {
	return 0, nil
}

func (*sessionBindingAuditRepo) List(context.Context, *service.AuditLogFilter) (*service.AuditLogList, error) {
	return nil, nil
}

func (*sessionBindingAuditRepo) GetByID(context.Context, int64) (*service.AuditLog, error) {
	return nil, nil
}

func (*sessionBindingAuditRepo) DeleteBefore(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}

func TestAuditLogMiddlewareIncludesRefreshSessionBindingMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &sessionBindingAuditRepo{}
	auditService := service.NewAuditLogService(repo, nil)
	auditService.Start()

	r := gin.New()
	require.NoError(t, r.SetTrustedProxies([]string{"172.24.149.0/24"}))
	r.Use(SessionBindingContext())
	r.POST("/api/v1/auth/refresh", gin.HandlerFunc(NewAuditLogMiddleware(auditService)), func(c *gin.Context) {
		expected := (&service.SessionBinding{
			IP:        "203.0.113.10",
			IPSource:  service.SessionBindingIPSourceTrustedForwarded,
			UserAgent: "test-agent",
		}).Fingerprint()
		binding := service.SessionBindingFromContext(c.Request.Context())
		require.Equal(t, service.SessionBindingClientIPMismatch, binding.Compare(expected))
		c.Status(http.StatusUnauthorized)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.RemoteAddr = "172.24.149.226:54321"
	req.Header.Set("X-Real-IP", "203.0.113.11")
	req.Header.Set("User-Agent", "test-agent")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)

	auditService.Stop()
	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Len(t, repo.logs, 1)
	require.Equal(t, service.AuditActionTokenRefresh, repo.logs[0].Action)
	require.Equal(t, "client_ip", repo.logs[0].Extra["mismatch_reason"])
	require.Equal(t, "trusted_forwarded", repo.logs[0].Extra["client_ip_source"])
	require.Equal(t, "172.24.149.226", repo.logs[0].Extra["peer_ip"])
}
