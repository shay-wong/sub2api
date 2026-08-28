package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUltrafastMultiplierMigration(t *testing.T) {
	content, err := FS.ReadFile("231_channel_ultrafast_multiplier.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS ultrafast_multiplier NUMERIC(12,6)")
	require.Contains(t, sql, "CHECK (ultrafast_multiplier IS NULL OR ultrafast_multiplier > 0)")
}
