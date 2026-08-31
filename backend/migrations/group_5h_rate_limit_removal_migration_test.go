package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroup5hRateLimitRemovalMigration(t *testing.T) {
	migration, err := FS.ReadFile("235_drop_group_5h_rate_limits.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(migration)), " ")
	dropTable := strings.Index(sql, "DROP TABLE IF EXISTS user_group_rate_limit_windows")
	dropColumn := strings.Index(sql, "DROP COLUMN IF EXISTS rate_limit_5h")
	require.NotEqual(t, -1, dropTable)
	require.Greater(t, dropColumn, dropTable)

	apiKeyMigration, err := FS.ReadFile("064_add_api_key_rate_limits.sql")
	require.NoError(t, err)
	require.Contains(t, string(apiKeyMigration), "ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS rate_limit_5h")
}
