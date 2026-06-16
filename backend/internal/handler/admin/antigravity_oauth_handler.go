package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type AntigravityOAuthHandler struct {
	antigravityOAuthService *service.AntigravityOAuthService
	adminService            service.AdminService
	permissionService       *service.PermissionService
}

func NewAntigravityOAuthHandler(antigravityOAuthService *service.AntigravityOAuthService, adminService service.AdminService, permissionService *service.PermissionService) *AntigravityOAuthHandler {
	return &AntigravityOAuthHandler{
		antigravityOAuthService: antigravityOAuthService,
		adminService:            adminService,
		permissionService:       permissionService,
	}
}

type AntigravityGenerateAuthURLRequest struct {
	ProxyID   *int64 `json:"proxy_id"`
	AccountID *int64 `json:"account_id"`
}

// GenerateAuthURL generates Google OAuth authorization URL
// POST /api/v1/admin/antigravity/oauth/auth-url
func (h *AntigravityOAuthHandler) GenerateAuthURL(c *gin.Context) {
	scope, scopeErr := resolveAdminAccessScope(c, h.permissionService)
	if scopeErr != nil {
		response.ErrorFrom(c, scopeErr)
		return
	}
	var req AntigravityGenerateAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}
	if err := scope.ensureOAuthProxyUse(c, h.adminService, req.AccountID, req.ProxyID); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	result, err := h.antigravityOAuthService.GenerateAuthURL(c.Request.Context(), req.ProxyID)
	if err != nil {
		response.InternalError(c, "生成授权链接失败: "+err.Error())
		return
	}

	response.Success(c, result)
}

type AntigravityExchangeCodeRequest struct {
	SessionID string `json:"session_id" binding:"required"`
	State     string `json:"state" binding:"required"`
	Code      string `json:"code" binding:"required"`
	ProxyID   *int64 `json:"proxy_id"`
	AccountID *int64 `json:"account_id"`
}

// ExchangeCode 用 authorization code 交换 token
// POST /api/v1/admin/antigravity/oauth/exchange-code
func (h *AntigravityOAuthHandler) ExchangeCode(c *gin.Context) {
	scope, scopeErr := resolveAdminAccessScope(c, h.permissionService)
	if scopeErr != nil {
		response.ErrorFrom(c, scopeErr)
		return
	}
	var req AntigravityExchangeCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}
	if err := scope.ensureOAuthProxyUse(c, h.adminService, req.AccountID, req.ProxyID); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	tokenInfo, err := h.antigravityOAuthService.ExchangeCode(c.Request.Context(), &service.AntigravityExchangeCodeInput{
		SessionID: req.SessionID,
		State:     req.State,
		Code:      req.Code,
		ProxyID:   req.ProxyID,
	})
	if err != nil {
		response.BadRequest(c, "Token 交换失败: "+err.Error())
		return
	}

	response.Success(c, tokenInfo)
}

// AntigravityRefreshTokenRequest represents the request for validating Antigravity refresh token
type AntigravityRefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
	ProxyID      *int64 `json:"proxy_id"`
	AccountID    *int64 `json:"account_id"`
}

// RefreshToken validates an Antigravity refresh token and returns full token info
// POST /api/v1/admin/antigravity/oauth/refresh-token
func (h *AntigravityOAuthHandler) RefreshToken(c *gin.Context) {
	scope, scopeErr := resolveAdminAccessScope(c, h.permissionService)
	if scopeErr != nil {
		response.ErrorFrom(c, scopeErr)
		return
	}
	var req AntigravityRefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}
	if err := scope.ensureOAuthProxyUse(c, h.adminService, req.AccountID, req.ProxyID); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	tokenInfo, err := h.antigravityOAuthService.ValidateRefreshToken(c.Request.Context(), req.RefreshToken, req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, tokenInfo)
}
