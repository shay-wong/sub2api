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

func TestProjectProfileScopeUnrestrictedKeepsProjectMembershipBoundary(t *testing.T) {
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
	require.Contains(t, normalized, "FROM project_members pm")
	require.Contains(t, normalized, "pm.project_id = 7")
	require.Contains(t, normalized, "ppb.resource_type = 'user'")
	require.Contains(t, normalized, "ppb.resource_id = usage_logs.user_id")
	require.Contains(t, normalized, "ppb.resource_type = 'group'")
	require.Contains(t, normalized, "ppb.resource_id = usage_logs.group_id")
	require.Contains(t, normalized, "ppb.resource_type = 'account'")
	require.Contains(t, normalized, "ppb.resource_id = usage_logs.account_id")
	require.Contains(t, normalized, "ppb.resource_type = 'api_key'")
	require.Contains(t, normalized, "ppb.resource_id = usage_logs.api_key_id")
}
