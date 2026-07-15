package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountDuplicateOperationIndexIsUniqueAndConcurrent(t *testing.T) {
	content, err := FS.ReadFile("177_account_duplicate_operation_index_notx.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_accounts_duplicate_operation_id")
	require.Contains(t, sql, "ON accounts ((extra ->> 'duplicate_operation_id'))")
	require.Contains(t, sql, "deleted_at IS NULL")
	require.Contains(t, sql, "extra ? 'duplicate_operation_id'")
	require.Contains(t, sql, "NULLIF(extra ->> 'duplicate_operation_id', '') IS NOT NULL")
}
