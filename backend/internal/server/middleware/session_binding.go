package middleware

import (
	"net"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SessionBindingContext 全局中间件：将请求的客户端 IP 与 User-Agent 注入
// request context，供 token 签发路径（登录 / 刷新 / OAuth 回调）读取并写入会话绑定，
// 同时作为审计日志、会话绑定校验的统一客户端 IP 来源。
// 会话绑定的客户端 IP 只通过 server.trusted_proxies 验证链读取，避免绑定到
// 瞬时 Docker / 反代 hop IP，也避免直连请求伪造原始转发头。
func SessionBindingContext(configs ...*config.Config) gin.HandlerFunc {
	var cfg *config.Config
	if len(configs) > 0 {
		cfg = configs[0]
	}
	return func(c *gin.Context) {
		if cfg != nil {
			forwardedIPSettings := cfg.ForwardedClientIPSettings()
			ip.SetForwardedIPSettings(c, forwardedIPSettings.TrustForwardedIP, forwardedIPSettings.Headers)
		}
		peerIP := requestPeerIP(c)
		clientIP := ip.GetTrustedClientIP(c)
		userAgent := normalizePersistentText(c.Request.UserAgent(), maxPersistentUserAgentBytes)
		c.Request.Header.Set("User-Agent", userAgent)
		binding := &service.SessionBinding{
			IP:        clientIP,
			IPSource:  sessionBindingIPSource(clientIP, peerIP),
			PeerIP:    peerIP,
			UserAgent: userAgent,
		}
		c.Request = c.Request.WithContext(service.WithSessionBinding(c.Request.Context(), binding))
		c.Next()
	}
}

// requestSessionBinding 返回当前请求的会话指纹，优先取 SessionBindingContext
// 注入的解析结果（保证与 token 签发路径取值一致）；注入缺失时使用安全回退。
func requestSessionBinding(c *gin.Context) *service.SessionBinding {
	if binding := service.SessionBindingFromContext(c.Request.Context()); binding != nil {
		return binding
	}
	peerIP := requestPeerIP(c)
	clientIP := ip.GetTrustedClientIP(c)
	return &service.SessionBinding{
		IP:        clientIP,
		IPSource:  sessionBindingIPSource(clientIP, peerIP),
		PeerIP:    peerIP,
		UserAgent: normalizePersistentText(c.Request.UserAgent(), maxPersistentUserAgentBytes),
	}
}

func sessionBindingIPSource(clientIP, peerIP string) service.SessionBindingIPSource {
	if clientIP != "" && clientIP != peerIP {
		return service.SessionBindingIPSourceTrustedForwarded
	}
	return service.SessionBindingIPSourcePeer
}

func requestPeerIP(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	remoteAddr := strings.TrimSpace(c.Request.RemoteAddr)
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}

// SecurityClientIP 返回当前请求用于安全敏感记录（审计日志等）的客户端 IP。
// 与会话绑定、API Key IP 限制共用同一套客户端 IP 来源。
func SecurityClientIP(c *gin.Context) string {
	if binding := service.SessionBindingFromContext(c.Request.Context()); binding != nil &&
		strings.TrimSpace(binding.IP) != "" {
		return binding.IP
	}
	return ip.GetTrustedClientIP(c)
}

// enforceSessionBinding 校验 access token 的会话指纹（IP/UA 绑定）。
// 指纹不匹配时：撤销该会话家族的所有 refresh token、写入审计安全事件、返回 401。
// 返回 false 表示请求已被中断。
//
// 兼容性：只有旧版 BindingHash 的会话在哈希未变化时允许 refresh 迁移；
// 所有指纹均为空的更早会话同样放行。
func enforceSessionBinding(
	c *gin.Context,
	authService *service.AuthService,
	settingService *service.SettingService,
	auditService *service.AuditLogService,
	claims *service.JWTClaims,
) bool {
	if settingService == nil || !settingService.IsSessionBindingEnabled(c.Request.Context()) {
		return true
	}
	if claims == nil {
		return true
	}
	binding := requestSessionBinding(c)
	expected := service.SessionBindingFingerprint{
		Hash:           claims.BindingHash,
		ClientIPHash:   claims.BindingClientIPHash,
		ClientIPSource: claims.BindingIPSource,
		UserAgentHash:  claims.BindingUserAgentHash,
	}
	mismatch := binding.Compare(expected)
	if mismatch == service.SessionBindingMatch {
		return true
	}

	if authService != nil {
		_ = authService.RevokeSessionFamily(c.Request.Context(), claims.SessionID)
	}
	if auditService != nil {
		uid := claims.UserID
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		auditService.Record(&service.AuditLog{
			ActorUserID: &uid,
			ActorEmail:  claims.Email,
			ActorRole:   claims.Role,
			AuthMethod:  service.AuditAuthMethodJWT,
			Action:      service.AuditActionSessionBindingMismatch,
			Method:      c.Request.Method,
			Path:        path,
			ClientIP:    binding.IP,
			UserAgent:   normalizePersistentText(c.Request.UserAgent(), maxPersistentUserAgentBytes),
			StatusCode:  401,
			Extra:       sessionBindingAuditMetadata(binding),
		})
	}
	AbortWithError(c, 401, "SESSION_BINDING_MISMATCH", sessionBindingMismatchMessage(mismatch))
	return false
}

func sessionBindingAuditMetadata(binding *service.SessionBinding) map[string]any {
	if binding == nil || binding.MismatchReason() == service.SessionBindingMatch {
		return nil
	}
	return map[string]any{
		"mismatch_reason":  string(binding.MismatchReason()),
		"client_ip_source": string(binding.IPSource),
		"peer_ip":          binding.PeerIP,
	}
}

func sessionBindingMismatchMessage(mismatch service.SessionBindingMismatch) string {
	switch mismatch {
	case service.SessionBindingClientIPMismatch:
		return "Session client network changed, please login again"
	case service.SessionBindingUserAgentMismatch:
		return "Session browser identity changed, please login again"
	case service.SessionBindingClientIPAndUserAgentMismatch:
		return "Session client network and browser identity changed, please login again"
	case service.SessionBindingPeerIPMismatch:
		return "Session transport peer changed, please check trusted proxy configuration"
	case service.SessionBindingPeerIPAndUserAgentMismatch:
		return "Session transport peer and browser identity changed, please login again"
	default:
		return "Session network fingerprint changed, please login again"
	}
}
