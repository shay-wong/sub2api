//go:build unit

package routes

import (
	"context"
	"net/http"
	"testing"

	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type mcpAccountReader struct {
	service.AdminService
	calls int
	t     *testing.T
	role  string
}

func (r *mcpAccountReader) GetAccount(ctx context.Context, id int64) (*service.Account, error) {
	role, ok := service.AdminRoleFromContext(ctx)
	require.True(r.t, ok)
	require.Equal(r.t, r.role, role)
	r.calls++
	return &service.Account{ID: id, Name: "bound-account", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, GroupIDs: []int64{10}}, nil
}

func TestMCPAdminDispatchPreservesPermissionsAndDirectAccountBindings(t *testing.T) {
	for _, tc := range []struct {
		name        string
		role        string
		permissions []string
		accountIDs  []int64
		wantStatus  int
		wantCalls   int
	}{
		{"super admin", service.RoleSuperAdmin, nil, nil, http.StatusOK, 1},
		{"direct account binding", service.RoleAdmin, []string{service.AdminPermissionAccountsWrite}, []int64{7}, http.StatusOK, 1},
		{"group does not grant account", service.RoleAdmin, []string{service.AdminPermissionAccountsWrite}, nil, http.StatusForbidden, 1},
		{"missing permission", service.RoleAdmin, nil, []int64{7}, http.StatusForbidden, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := mcpRouteHandlers()
			accounts := &mcpAccountReader{t: t, role: tc.role}
			scopeRepo := &openCodeGoUsagePermissionRepo{scope: service.AdminResourceScope{Mode: service.AdminResourceScopeRestricted, GroupIDs: []int64{10}, AccountIDs: tc.accountIDs}}
			permissions := service.NewPermissionService(scopeRepo, nil, nil)
			h.Admin.Account = adminhandler.NewAccountHandler(accounts, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, permissions)
			principal := &service.MCPPrincipal{UserID: 8, GrantID: "scope", Role: tc.role, Scope: service.MCPScopeAdmin, ReadOnly: true, AdminPermissions: tc.permissions}
			endpoint := newMCPRouteTestEndpoint(h, principal, nil, middleware.AuditLogMiddleware(mcpRouteNext))
			result := callMCPRouteTool(t, endpoint, "admin_get_account", map[string]any{"id": 7})
			require.Equal(t, tc.wantStatus, result.Result.Payload.HTTPStatus)
			require.Equal(t, tc.wantCalls, accounts.calls)
			if tc.wantStatus != http.StatusOK {
				require.NotContains(t, string(result.Result.Payload.Result), "bound-account")
			}
			if tc.role == service.RoleAdmin && tc.wantStatus == http.StatusOK {
				principal.AdminPermissions = nil
				result = callMCPRouteTool(t, endpoint, "admin_get_account", map[string]any{"id": 7})
				require.Equal(t, http.StatusForbidden, result.Result.Payload.HTTPStatus)
				require.Equal(t, 1, accounts.calls)
				principal.AdminPermissions = tc.permissions
				scopeRepo.scope.AccountIDs = nil
				result = callMCPRouteTool(t, endpoint, "admin_get_account", map[string]any{"id": 7})
				require.Equal(t, http.StatusForbidden, result.Result.Payload.HTTPStatus)
			}
		})
	}
}
