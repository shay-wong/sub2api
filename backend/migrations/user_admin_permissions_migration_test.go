package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserAdminPermissionsMigrations(t *testing.T) {
	expand, err := FS.ReadFile("233_add_user_admin_permissions.sql")
	require.NoError(t, err)
	expandSQL := strings.Join(strings.Fields(string(expand)), " ")
	require.Contains(t, expandSQL, "admin_permissions JSONB NOT NULL DEFAULT '[]'::jsonb")
	require.Contains(t, expandSQL, "CHECK (jsonb_typeof(admin_permissions) = 'array')")

	backfill, err := FS.ReadFile("234_backfill_user_admin_permissions.sql")
	require.NoError(t, err)
	backfillSQL := strings.Join(strings.Fields(string(backfill)), " ")
	require.Contains(t, backfillSQL, "pm.role = 'admin'")
	require.Contains(t, backfillSQL, "pm.status = 'active'")
	require.Contains(t, backfillSQL, "jsonb_array_length(aam.scopes) = 0")
	require.Contains(t, backfillSQL, "NOT (aam.scopes ? '__none__')")
	require.Contains(t, backfillSQL, "jsonb_agg(DISTINCT permission ORDER BY permission)")
	require.Contains(t, backfillSQL, "u.role <> 'super_admin'")
}
