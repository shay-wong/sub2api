//go:build unit

package service

import "testing"

// TestSettingKeyDefaultPlatformQuotas 验证新的系统层 JSON key 常量值正确。
func TestSettingKeyDefaultPlatformQuotas(t *testing.T) {
	if SettingKeyDefaultPlatformQuotas != "default_platform_quotas" {
		t.Errorf("SettingKeyDefaultPlatformQuotas = %q, want %q",
			SettingKeyDefaultPlatformQuotas, "default_platform_quotas")
	}
}

// TestSettingKeyAuthSourcePlatformQuotas 验证新的 auth-source JSON key 函数返回值正确。
func TestSettingKeyAuthSourcePlatformQuotas(t *testing.T) {
	if got := SettingKeyAuthSourcePlatformQuotas("email"); got != "auth_source_default_email_platform_quotas" {
		t.Fatalf("got %q, want %q", got, "auth_source_default_email_platform_quotas")
	}
	if got := SettingKeyAuthSourcePlatformQuotas("dingtalk"); got != "auth_source_default_dingtalk_platform_quotas" {
		t.Fatalf("got %q, want %q", got, "auth_source_default_dingtalk_platform_quotas")
	}
}

func TestRoleHierarchyHelpers(t *testing.T) {
	tests := []struct {
		role             string
		wantAdmin        bool
		wantOperator     bool
		wantUserAccess   bool
		wantAdminConsole bool
	}{
		{role: RoleSuperAdmin, wantAdmin: true, wantUserAccess: true, wantAdminConsole: true},
		{role: RoleAdmin, wantAdmin: true, wantUserAccess: true, wantAdminConsole: true},
		{role: RoleOperator, wantOperator: true},
		{role: RoleUser, wantUserAccess: true},
		{role: ""},
		{role: "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.role, func(t *testing.T) {
			if got := RoleIsAdmin(tc.role); got != tc.wantAdmin {
				t.Fatalf("RoleIsAdmin(%q) = %v, want %v", tc.role, got, tc.wantAdmin)
			}
			if got := RoleIsOperator(tc.role); got != tc.wantOperator {
				t.Fatalf("RoleIsOperator(%q) = %v, want %v", tc.role, got, tc.wantOperator)
			}
			if got := RoleHasUserAccess(tc.role); got != tc.wantUserAccess {
				t.Fatalf("RoleHasUserAccess(%q) = %v, want %v", tc.role, got, tc.wantUserAccess)
			}
			if got := RoleCanAccessAdminConsole(tc.role); got != tc.wantAdminConsole {
				t.Fatalf("RoleCanAccessAdminConsole(%q) = %v, want %v", tc.role, got, tc.wantAdminConsole)
			}
		})
	}
}
