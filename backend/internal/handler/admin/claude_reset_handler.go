package admin

import (
	"context"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type claudeResetReader interface {
	Query(context.Context, int64) (*service.ClaudeResetCredits, error)
	Redeem(context.Context, int64, string) (*service.ClaudeResetOutcome, error)
}

func (h *AccountHandler) SetClaudeResetCreditService(s *service.ClaudeResetCreditService) {
	h.claudeResetCredits = s
}

func (h *AccountHandler) claudeResetAccountID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return 0, false
	}
	scope, err := resolveAdminAccessScope(c, h.permissionService)
	if err != nil {
		response.ErrorFrom(c, err)
		return 0, false
	}
	if err := h.ensureAccountInScope(c, scope, id); err != nil {
		response.ErrorFrom(c, err)
		return 0, false
	}
	if h.claudeResetCredits == nil {
		response.Error(c, 503, "Claude reset service unavailable")
		return 0, false
	}
	return id, true
}

func (h *AccountHandler) ClaudeResetCredits(c *gin.Context) {
	id, ok := h.claudeResetAccountID(c)
	if !ok {
		return
	}
	status, err := h.claudeResetCredits.Query(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

// RedeemClaudeResetCredit consumes the upstream next reset credit. The request has
// no body: the server picks the grant; the Idempotency-Key header identifies one
// operator confirmation and replays its outcome.
func (h *AccountHandler) RedeemClaudeResetCredit(c *gin.Context) {
	id, ok := h.claudeResetAccountID(c)
	if !ok {
		return
	}
	result, err := h.claudeResetCredits.Redeem(c.Request.Context(), id, c.GetHeader("Idempotency-Key"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
