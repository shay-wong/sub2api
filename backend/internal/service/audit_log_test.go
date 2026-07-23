package service

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestMaskAuditCredential(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"short", "abc", "****"},
		{"boundary_14", "12345678901234", "****"},
		{"long", "sk-ant-api03-abcdefghijklmnop1234", "sk-ant****1234"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MaskAuditCredential(tc.in)
			if got != tc.want {
				t.Fatalf("MaskAuditCredential(%q) = %q, want %q", tc.in, got, tc.want)
			}
			// 掩码结果绝不能包含原始凭证的中间部分。
			if len(tc.in) > 14 && strings.Contains(got, tc.in) {
				t.Fatalf("masked value leaks full credential: %q", got)
			}
		})
	}
}

func TestRedactAuditBody_JSONRedactsSecrets(t *testing.T) {
	raw := []byte(`{
		"name": "acc1",
		"base_url": "https://evil.example.com",
		"credentials": {"api_key": "sk-secret-123", "base_url": "https://evil.example.com"},
		"new_password": "hunter2",
		"totp_code": "123456",
		"nested": [{"access_token": "tok_abc"}]
	}`)
	out := RedactAuditBody(raw, "application/json")

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}

	// 敏感字段被擦除。
	for _, secret := range []string{"sk-secret-123", "hunter2", "123456", "tok_abc"} {
		if strings.Contains(out, secret) {
			t.Fatalf("redacted body still contains secret %q: %s", secret, out)
		}
	}
	// 非敏感字段（base_url、name）保留以便追责。
	if !strings.Contains(out, "evil.example.com") {
		t.Fatalf("base_url should be preserved for accountability: %s", out)
	}
	if !strings.Contains(out, "acc1") {
		t.Fatalf("name should be preserved: %s", out)
	}
}

// 裸键 "session"（Ollama Cloud 会话保存的请求体字段）值整体就是浏览器 Cookie 明文，
// 必须命中键级脱敏；session_id 等运行态标识不受影响，保留以便追责。
func TestRedactAuditBody_BareSessionKeyRedacted(t *testing.T) {
	raw := []byte(`{"session": "wos-session=cookie-canary", "session_id": "sid-visible"}`)
	out := RedactAuditBody(raw, "application/json")

	if strings.Contains(out, "cookie-canary") {
		t.Fatalf("redacted body still contains the session cookie: %s", out)
	}
	if !strings.Contains(out, "sid-visible") {
		t.Fatalf("session_id should be preserved for accountability: %s", out)
	}
}

// TestRedactAuditBody_AuthoritativeTablesSynced 覆盖曾经漏网的凭证字段：
// 账号 credentials 敏感子键、支付渠道无分隔符密钥、字符串值内嵌凭证的 proxy_key / custom_key，
// 以及 camelCase 等命名变体（归一化比对）。
func TestRedactAuditBody_AuthoritativeTablesSynced(t *testing.T) {
	raw := []byte(`{
		"credentials": {
			"session_key": "sk-session-aaa",
			"service_account_json": "{\"private_key\":\"pem-body-bbb\"}",
			"service_account": "sa-blob-ccc"
		},
		"proxy_key": "socks5|1.2.3.4|1080|proxyuser|proxypass-ddd",
		"custom_key": "sk-custom-eee",
		"config": {
			"pkey": "easypay-merchant-fff",
			"privateKey": "alipay-pem-ggg",
			"apiv3key": "wxpay-v3-hhh",
			"SecretKey": "stripe-sk-iii",
			"webhookSecret": "whsec-jjj"
		},
		"provider_key": "stripe",
		"name": "instance-1"
	}`)
	out := RedactAuditBody(raw, "application/json")

	for _, secret := range []string{
		"sk-session-aaa", "pem-body-bbb", "sa-blob-ccc",
		"proxypass-ddd", "sk-custom-eee",
		"easypay-merchant-fff", "alipay-pem-ggg", "wxpay-v3-hhh",
		"stripe-sk-iii", "whsec-jjj",
	} {
		if strings.Contains(out, secret) {
			t.Fatalf("redacted body still contains secret %q: %s", secret, out)
		}
	}
	// provider_key 是渠道标识而非密钥，必须保留以便追责。
	if !strings.Contains(out, `"provider_key":"stripe"`) {
		t.Fatalf("provider_key should be preserved for accountability: %s", out)
	}
	if !strings.Contains(out, "instance-1") {
		t.Fatalf("name should be preserved: %s", out)
	}
}

// SensitiveCredentialKeys 中的每个键都必须被审计脱敏判定命中（防两表漂移的守卫）。
func TestAuditSensitiveKeys_CoverCredentialTable(t *testing.T) {
	for _, k := range SensitiveCredentialKeys {
		if !isAuditSensitiveBodyKey(k) {
			t.Fatalf("credential key %q is not covered by audit redaction", k)
		}
	}
	for provider, fields := range providerSensitiveConfigFields {
		for k := range fields {
			if !isAuditSensitiveBodyKey(k) {
				t.Fatalf("payment provider %q sensitive field %q is not covered by audit redaction", provider, k)
			}
		}
	}
}

func TestRedactAuditBody_NonJSONOmitted(t *testing.T) {
	out := RedactAuditBody([]byte("username=admin&password=secret"), "application/x-www-form-urlencoded")
	if strings.Contains(out, "secret") {
		t.Fatalf("non-json body must not leak content: %s", out)
	}
	if !strings.Contains(out, "omitted") {
		t.Fatalf("expected omission marker, got: %s", out)
	}
}

func TestRedactAuditBody_Empty(t *testing.T) {
	if got := RedactAuditBody(nil, "application/json"); got != "" {
		t.Fatalf("expected empty for nil body, got %q", got)
	}
}

func TestRedactAuditBody_TruncatedUnicodeRemainsValidUTF8(t *testing.T) {
	raw := []byte(`{"message":"` + strings.Repeat("中", 6000) + `"}`)
	out := RedactAuditBody(raw, "application/json")

	if !utf8.ValidString(out) {
		t.Fatal("truncated audit body contains invalid UTF-8")
	}
	if !strings.HasSuffix(out, "...<truncated>") {
		t.Fatalf("expected truncation marker, got suffix %q", out[len(out)-32:])
	}
}

func TestSessionBindingHash(t *testing.T) {
	a := &SessionBinding{IP: "1.2.3.4", IPSource: SessionBindingIPSourceTrustedForwarded, PeerIP: "172.24.149.226", UserAgent: "Mozilla/5.0"}
	b := &SessionBinding{IP: "1.2.3.4", IPSource: SessionBindingIPSourceTrustedForwarded, PeerIP: "172.24.149.227", UserAgent: "Mozilla/5.0"}
	if a.Hash() != b.Hash() {
		t.Fatalf("proxy peer changes must not change the client binding hash")
	}
	if a.Hash() == "" {
		t.Fatalf("non-empty binding must produce non-empty hash")
	}
	if mismatch := b.Mismatch(a.Fingerprint()); mismatch != SessionBindingMatch {
		t.Fatalf("proxy peer changes must match, got %q", mismatch)
	}

	// 可信客户端 IP 变化与 UA 变化必须可以分别诊断。
	c := &SessionBinding{IP: "5.6.7.8", IPSource: SessionBindingIPSourceTrustedForwarded, UserAgent: "Mozilla/5.0"}
	if mismatch := c.Mismatch(a.Fingerprint()); mismatch != SessionBindingClientIPMismatch {
		t.Fatalf("client IP change mismatch = %q, want %q", mismatch, SessionBindingClientIPMismatch)
	}
	d := &SessionBinding{IP: "1.2.3.4", IPSource: SessionBindingIPSourceTrustedForwarded, UserAgent: "curl/8.0"}
	if mismatch := d.Mismatch(a.Fingerprint()); mismatch != SessionBindingUserAgentMismatch {
		t.Fatalf("user agent change mismatch = %q, want %q", mismatch, SessionBindingUserAgentMismatch)
	}
	missingIP := &SessionBinding{IPSource: SessionBindingIPSourceTrustedForwarded, UserAgent: "Mozilla/5.0"}
	if mismatch := missingIP.Mismatch(a.Fingerprint()); mismatch != SessionBindingClientIPMismatch {
		t.Fatalf("missing client IP mismatch = %q, want %q", mismatch, SessionBindingClientIPMismatch)
	}
	missingUserAgent := &SessionBinding{IP: "1.2.3.4", IPSource: SessionBindingIPSourceTrustedForwarded}
	if mismatch := missingUserAgent.Mismatch(a.Fingerprint()); mismatch != SessionBindingUserAgentMismatch {
		t.Fatalf("missing user agent mismatch = %q, want %q", mismatch, SessionBindingUserAgentMismatch)
	}

	peerIssued := &SessionBinding{IP: "172.24.149.226", IPSource: SessionBindingIPSourcePeer, UserAgent: "Mozilla/5.0"}
	peerCurrent := &SessionBinding{IP: "172.24.149.227", IPSource: SessionBindingIPSourcePeer, UserAgent: "Mozilla/5.0"}
	if mismatch := peerCurrent.Mismatch(peerIssued.Fingerprint()); mismatch != SessionBindingPeerIPMismatch {
		t.Fatalf("peer IP change mismatch = %q, want %q", mismatch, SessionBindingPeerIPMismatch)
	}

	// 旧版只有合并哈希时，仅允许未变化的会话继续并在 refresh 时迁移。
	legacyExpected := SessionBindingFingerprint{Hash: peerIssued.Hash()}
	if mismatch := peerIssued.Mismatch(legacyExpected); mismatch != SessionBindingMatch {
		t.Fatalf("unchanged legacy fingerprint must match, got %q", mismatch)
	}
	if mismatch := peerCurrent.Mismatch(legacyExpected); mismatch != SessionBindingLegacyMismatch {
		t.Fatalf("changed legacy fingerprint mismatch = %q, want %q", mismatch, SessionBindingLegacyMismatch)
	}

	// 空指纹 → 空哈希（旧 token 兼容）。
	empty := &SessionBinding{}
	if empty.Hash() != "" {
		t.Fatalf("empty binding must hash to empty string")
	}
	var nilBinding *SessionBinding
	if nilBinding.Hash() != "" {
		t.Fatalf("nil binding must hash to empty string")
	}
}

func TestParseAuditLogRetentionDays(t *testing.T) {
	cases := map[string]int{
		"":       defaultAuditLogRetentionDays,
		"abc":    defaultAuditLogRetentionDays,
		"90":     90,
		"0":      0,
		"-1":     0,
		"  30  ": 30,
	}
	for in, want := range cases {
		if got := parseAuditLogRetentionDays(in); got != want {
			t.Fatalf("parseAuditLogRetentionDays(%q) = %d, want %d", in, got, want)
		}
	}
}
