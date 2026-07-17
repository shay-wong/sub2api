package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration182AddsPersistentAuditClearState(t *testing.T) {
	content, err := FS.ReadFile("182_audit_log_clear_state.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ALTER SEQUENCE audit_logs_id_seq CACHE 1 NO CYCLE")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS audit_log_state")
	require.Contains(t, sql, "clear_id BIGINT NOT NULL DEFAULT 0")
}
