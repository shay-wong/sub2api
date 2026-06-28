package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestResolveProjectIDForCreatePrefersContextProject(t *testing.T) {
	ctx := service.WithProjectID(context.Background(), 7)

	projectID, err := resolveProjectIDForCreate(ctx, nil, 99)

	require.NoError(t, err)
	require.Equal(t, int64(7), projectID)
}

func TestResolveProjectIDForCreateUsesRequestedWithoutContext(t *testing.T) {
	projectID, err := resolveProjectIDForCreate(context.Background(), nil, 99)

	require.NoError(t, err)
	require.Equal(t, int64(99), projectID)
}

func TestProjectProfileScopeUnrestrictedKeepsProjectIDBoundaryOnlyWhenRequired(t *testing.T) {
	clause := projectProfileScopeSQL(7, projectSQLScopeResources{
		ProjectID:        "usage_logs.project_id",
		RequireProjectID: true,
		UserID:           "usage_logs.user_id",
		GroupID:          "usage_logs.group_id",
		AccountID:        "usage_logs.account_id",
		APIKeyID:         "usage_logs.api_key_id",
	})
	normalized := normalizeSQLWhitespace(clause)

	require.Contains(t, normalized, "pp.mode = 'unrestricted'")
	require.Contains(t, normalized, "usage_logs.project_id = 7")
	require.Contains(t, normalized, "ppb.resource_type = 'group'")
	require.Contains(t, normalized, "ppb.resource_id = usage_logs.group_id")
	require.Contains(t, normalized, "ppb.resource_type = 'account'")
	require.Contains(t, normalized, "ppb.resource_id = usage_logs.account_id")
	require.NotContains(t, normalized, "ppb.resource_type = 'user'")
	require.NotContains(t, normalized, "ppb.resource_type = 'api_key'")
}

func TestUsageLogProjectScopeUsesBoundResourcesWithoutHardProjectIDBoundary(t *testing.T) {
	clause := projectProfileScopeSQL(7, usageLogSQLScopeResources("usage_logs"))
	normalized := normalizeSQLWhitespace(clause)

	require.Contains(t, normalized, "FROM project_members pm")
	require.Contains(t, normalized, "pm.project_id = 7")
	require.Contains(t, normalized, "pm.user_id = usage_logs.user_id")
	require.Contains(t, normalized, "pp.mode = 'unrestricted'")
	require.Contains(t, normalized, "ppb.resource_type = 'group'")
	require.Contains(t, normalized, "ppb.resource_id = usage_logs.group_id")
	require.Contains(t, normalized, "ppb.resource_type = 'account'")
	require.Contains(t, normalized, "ppb.resource_id = usage_logs.account_id")
	require.Contains(t, normalized, "ppb.resource_type = 'subscription'")
	require.Contains(t, normalized, "ppb.resource_id = usage_logs.subscription_id")
	require.NotContains(t, normalized, "usage_logs.project_id = 7")
}

func TestOpsErrorProjectScopeUsesBoundResourcesWithoutHardProjectIDBoundary(t *testing.T) {
	clause := projectProfileScopeSQL(7, opsErrorSQLScopeResources("ops_error_logs"))
	normalized := normalizeSQLWhitespace(clause)

	require.Contains(t, normalized, "FROM project_members pm")
	require.Contains(t, normalized, "pm.project_id = 7")
	require.Contains(t, normalized, "pm.user_id = ops_error_logs.user_id")
	require.Contains(t, normalized, "pp.mode = 'unrestricted'")
	require.Contains(t, normalized, "ppb.resource_type = 'group'")
	require.Contains(t, normalized, "ppb.resource_id = ops_error_logs.group_id")
	require.Contains(t, normalized, "ppb.resource_type = 'account'")
	require.Contains(t, normalized, "ppb.resource_id = ops_error_logs.account_id")
	require.NotContains(t, normalized, "ops_error_logs.project_id = 7")
}

func TestAPIKeyProjectScopeRequiresSingleProjectAndMemberOwner(t *testing.T) {
	clause := projectProfileScopeSQL(7, apiKeySQLScopeResources("api_keys"))
	normalized := normalizeSQLWhitespace(clause)

	require.Contains(t, normalized, "api_keys.project_id = 7")
	require.Contains(t, normalized, "FROM project_members pm")
	require.Contains(t, normalized, "pm.project_id = 7")
	require.Contains(t, normalized, "pm.user_id = api_keys.user_id")
	require.NotContains(t, normalized, "pm.status")
	require.NotContains(t, normalized, "project_profile_bindings")
	require.NotContains(t, normalized, "pp.mode = 'unrestricted'")
}

func TestAPIKeyProjectPredicateRequiresActiveMemberForAuthContext(t *testing.T) {
	predicate := projectScopedAPIKeyPredicate(service.WithRequireActiveProjectMember(service.WithProjectID(context.Background(), 7)))

	require.Len(t, predicate, 1)
	require.True(t, service.RequireActiveProjectMemberFromContext(service.WithRequireActiveProjectMember(context.Background())))
}

func TestAccountProjectScopeDoesNotExpandFromConfiguredGroups(t *testing.T) {
	clause := projectProfileScopeSQL(7, projectSQLScopeResources{AccountID: "accounts.id"})
	normalized := normalizeSQLWhitespace(clause)

	require.Contains(t, normalized, "ppb.resource_type = 'account'")
	require.Contains(t, normalized, "ppb.resource_id = accounts.id")
	require.NotContains(t, normalized, "account_groups")
	require.NotContains(t, normalized, "ppb.resource_type = 'group'")
}

func TestProxyProjectScopeUsesProfileBindingWithoutHardProjectIDBoundary(t *testing.T) {
	clause := projectProfileScopeSQL(7, projectSQLScopeResources{ProxyID: "proxies.id"})
	normalized := normalizeSQLWhitespace(clause)

	require.Contains(t, normalized, "pp.mode = 'unrestricted'")
	require.Contains(t, normalized, "ppb.resource_type = 'proxy'")
	require.Contains(t, normalized, "ppb.resource_id = proxies.id")
	require.NotContains(t, normalized, "proxies.project_id = 7")
}

func TestProjectUserGroupScopeRequiresMemberAndConfiguredGroup(t *testing.T) {
	clause := projectUserGroupScopeSQL(7, "u.id", "g.id")
	normalized := normalizeSQLWhitespace(clause)

	require.Contains(t, normalized, "FROM project_members pm")
	require.Contains(t, normalized, "pm.user_id = u.id")
	require.Contains(t, normalized, "pp.mode = 'unrestricted'")
	require.Contains(t, normalized, "ppb.resource_type = 'group'")
	require.Contains(t, normalized, "ppb.resource_id = g.id")
	require.NotContains(t, normalized, "ppb.resource_type = 'user'")
}
