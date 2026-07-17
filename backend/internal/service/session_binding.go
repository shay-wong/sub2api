package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ErrSessionBindingMismatch 会话绑定的 IP/UA 发生变化，会话已失效。
var ErrSessionBindingMismatch = infraerrors.Unauthorized("SESSION_BINDING_MISMATCH", "session network fingerprint changed, please login again")

// SessionBinding 会话指纹：登录时的客户端 IP 与 User-Agent。
// 会话绑定开启时，两者任一变化即导致会话失效（防止凭证被盗后异地重放）。
type SessionBinding struct {
	IP        string
	IPSource  SessionBindingIPSource
	PeerIP    string
	UserAgent string

	mismatchReason SessionBindingMismatch
}

type SessionBindingIPSource string

const (
	SessionBindingIPSourcePeer             SessionBindingIPSource = "peer"
	SessionBindingIPSourceTrustedForwarded SessionBindingIPSource = "trusted_forwarded"
)

type SessionBindingFingerprint struct {
	Hash           string // 旧版合并指纹，仅用于兼容已签发 token/Redis 数据。
	ClientIPHash   string
	ClientIPSource SessionBindingIPSource
	UserAgentHash  string
}

type SessionBindingMismatch string

const (
	SessionBindingMatch                        SessionBindingMismatch = ""
	SessionBindingClientIPMismatch             SessionBindingMismatch = "client_ip"
	SessionBindingPeerIPMismatch               SessionBindingMismatch = "transport_peer_ip"
	SessionBindingUserAgentMismatch            SessionBindingMismatch = "user_agent"
	SessionBindingClientIPAndUserAgentMismatch SessionBindingMismatch = "client_ip_and_user_agent"
	SessionBindingPeerIPAndUserAgentMismatch   SessionBindingMismatch = "transport_peer_ip_and_user_agent"
	SessionBindingLegacyMismatch               SessionBindingMismatch = "legacy_fingerprint"
)

// Hash 计算绑定指纹哈希（IP 与 UA 合并，任一变化哈希即变化）。
func (b *SessionBinding) Hash() string {
	if b == nil {
		return ""
	}
	ip := strings.TrimSpace(b.IP)
	ua := strings.TrimSpace(b.UserAgent)
	if ip == "" && ua == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(ip + "\n" + ua))
	return hex.EncodeToString(sum[:16])
}

func (b *SessionBinding) Fingerprint() SessionBindingFingerprint {
	if b == nil {
		return SessionBindingFingerprint{}
	}
	return SessionBindingFingerprint{
		Hash:           b.Hash(),
		ClientIPHash:   hashSessionBindingPart(b.IP),
		ClientIPSource: b.IPSource,
		UserAgentHash:  hashSessionBindingPart(b.UserAgent),
	}
}

// Compare 比较当前请求与已签发指纹，并保存结果供请求结束后的审计日志使用。
func (b *SessionBinding) Compare(expected SessionBindingFingerprint) SessionBindingMismatch {
	mismatch := b.Mismatch(expected)
	if b != nil {
		b.mismatchReason = mismatch
	}
	return mismatch
}

// Mismatch 只计算指纹差异，不修改请求状态。
func (b *SessionBinding) Mismatch(expected SessionBindingFingerprint) SessionBindingMismatch {
	current := b.Fingerprint()
	ipMismatch := expected.ClientIPHash != "" && expected.ClientIPHash != current.ClientIPHash
	uaMismatch := expected.UserAgentHash != "" && expected.UserAgentHash != current.UserAgentHash

	ipMismatchReason := SessionBindingClientIPMismatch
	if expected.ClientIPSource != SessionBindingIPSourceTrustedForwarded ||
		current.ClientIPSource != SessionBindingIPSourceTrustedForwarded {
		ipMismatchReason = SessionBindingPeerIPMismatch
	}

	switch {
	case ipMismatch && uaMismatch:
		if ipMismatchReason == SessionBindingPeerIPMismatch {
			return SessionBindingPeerIPAndUserAgentMismatch
		}
		return SessionBindingClientIPAndUserAgentMismatch
	case ipMismatch:
		return ipMismatchReason
	case uaMismatch:
		return SessionBindingUserAgentMismatch
	case expected.ClientIPHash != "" || expected.UserAgentHash != "":
		return SessionBindingMatch
	case expected.Hash != "" && expected.Hash != current.Hash:
		return SessionBindingLegacyMismatch
	default:
		return SessionBindingMatch
	}
}

// MismatchReason 返回本请求最近一次 Compare 的结果。
func (b *SessionBinding) MismatchReason() SessionBindingMismatch {
	if b == nil {
		return SessionBindingMatch
	}
	return b.mismatchReason
}

func hashSessionBindingPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:16])
}

type sessionBindingCtxKey struct{}

// WithSessionBinding 将会话指纹注入 context（由 HTTP 入口中间件调用）。
func WithSessionBinding(ctx context.Context, binding *SessionBinding) context.Context {
	if binding == nil {
		return ctx
	}
	return context.WithValue(ctx, sessionBindingCtxKey{}, binding)
}

// SessionBindingFromContext 从 context 提取会话指纹；不存在时返回 nil。
func SessionBindingFromContext(ctx context.Context) *SessionBinding {
	if ctx == nil {
		return nil
	}
	binding, _ := ctx.Value(sessionBindingCtxKey{}).(*SessionBinding)
	return binding
}

func sessionBindingFingerprintFromContext(ctx context.Context) SessionBindingFingerprint {
	return SessionBindingFromContext(ctx).Fingerprint()
}

func sessionBindingMismatchFromContext(ctx context.Context, expected SessionBindingFingerprint) SessionBindingMismatch {
	return SessionBindingFromContext(ctx).Compare(expected)
}
