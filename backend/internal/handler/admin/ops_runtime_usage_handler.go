package admin

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// GetUsageRecordRuntime returns process-global usage record queue and persistence health.
// GET /api/v1/admin/ops/runtime/usage-record
func (h *OpsHandler) GetUsageRecordRuntime(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	stats, err := h.opsService.GetUsageRecordRuntimeStats(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, stats)
}
