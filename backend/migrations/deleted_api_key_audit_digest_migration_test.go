package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeletedAPIKeyAuditDigestMigrationRemovesPlaintextRetention(t *testing.T) {
	content, err := FS.ReadFile("185_deleted_api_key_audit_digest.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(strings.ToLower(string(content))), " ")
	require.Contains(t, sql, "add column if not exists key_digest varchar(64)")
	require.Contains(t, sql, "new.key_digest := encode(sha256(convert_to(new.key, 'utf8')), 'hex')")
	require.Contains(t, sql, "new.key := ''")
	require.Contains(t, sql, "alter column key_digest set not null")
	require.Contains(t, sql, "deletedapikeyaudit_key_digest")
}
