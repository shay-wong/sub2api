package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPasswordAuthDisabledMigrationPreservesUnknownLegacyState(t *testing.T) {
	content, err := FS.ReadFile("193_add_users_password_auth_disabled.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ADD COLUMN password_auth_disabled BOOLEAN")
	require.NotContains(t, sql, "NOT NULL")
	require.NotContains(t, sql, "UPDATE users")
}
