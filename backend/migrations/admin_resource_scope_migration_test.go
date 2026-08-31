package migrations_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdminResourceScopeMigrationKeepsExistingAdminsUnrestricted(t *testing.T) {
	content, err := os.ReadFile("236_admin_resource_scopes.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(content))

	require.Contains(t, sql, "create table if not exists admin_resource_scopes")
	require.Contains(t, sql, "create table if not exists admin_resource_bindings")
	require.Contains(t, sql, "check (mode in ('all', 'restricted'))")
	require.Contains(t, sql, "check (resource_type in ('group', 'account', 'proxy', 'subscription'))")
	require.Contains(t, sql, "where role = 'admin'")
	require.Contains(t, sql, "'all'")
}
