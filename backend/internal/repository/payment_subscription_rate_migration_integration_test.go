//go:build integration

package repository

import (
	"context"
	"database/sql"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMigrateLegacySubscriptionCNYRate(t *testing.T) {
	migrationSQL, err := dbmigrations.FS.ReadFile("250_migrate_subscription_cny_payment_multiplier.sql")
	require.NoError(t, err)
	ptr := func(value string) *string { return &value }
	for _, tt := range []struct {
		name      string
		canonical *string
		legacy    *string
		want      *string
	}{
		{name: "no settings"},
		{name: "empty legacy", legacy: ptr("")},
		{name: "whitespace legacy", legacy: ptr(" \t\n")},
		{name: "positive legacy", legacy: ptr("0.2"), want: ptr("5")},
		{name: "legacy reciprocal rounding", legacy: ptr("0.3"), want: ptr("3.333333")},
		{name: "legacy one", legacy: ptr("1"), want: ptr("1")},
		{name: "legacy zero", legacy: ptr("0"), want: ptr("1")},
		{name: "legacy negative", legacy: ptr("-2"), want: ptr("1")},
		{name: "legacy NaN", legacy: ptr("NaN"), want: ptr("1")},
		{name: "legacy infinity", legacy: ptr("+Inf"), want: ptr("1")},
		{name: "legacy invalid", legacy: ptr("abc"), want: ptr("1")},
		{name: "legacy parser rejects surrounding whitespace", legacy: ptr(" 0.2 "), want: ptr("1")},
		{name: "legacy reciprocal overflows", legacy: ptr("5e-324"), want: ptr("0")},
		{name: "legacy reciprocal rounds to zero", legacy: ptr("1e308"), want: ptr("0")},
		{name: "zero canonical wins", canonical: ptr("0"), legacy: ptr("0.2"), want: ptr("0")},
		{name: "configured canonical wins", canonical: ptr("7.15"), legacy: ptr("0.2"), want: ptr("7.15")},
		{name: "canonical whitespace preserved", canonical: ptr(" 7.15 "), legacy: ptr("0.2"), want: ptr(" 7.15 ")},
		{name: "invalid canonical backfilled", canonical: ptr("abc"), legacy: ptr("0.2"), want: ptr("5")},
		{name: "negative canonical backfilled", canonical: ptr("-1"), legacy: ptr("0.2"), want: ptr("5")},
		{name: "infinite canonical backfilled", canonical: ptr("Infinity"), legacy: ptr("0.2"), want: ptr("5")},
		{name: "invalid canonical without legacy preserved", canonical: ptr("abc"), want: ptr("abc")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tx := testTx(t)
			ctx := context.Background()
			_, err := tx.ExecContext(ctx, `DELETE FROM settings WHERE key IN ('SUBSCRIPTION_USD_TO_CNY_RATE', 'SUBSCRIPTION_CNY_PAYMENT_MULTIPLIER')`)
			require.NoError(t, err)
			for key, value := range map[string]*string{
				"SUBSCRIPTION_USD_TO_CNY_RATE":        tt.canonical,
				"SUBSCRIPTION_CNY_PAYMENT_MULTIPLIER": tt.legacy,
			} {
				if value != nil {
					_, err = tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ($1, $2)`, key, *value)
					require.NoError(t, err)
				}
			}

			for range 2 {
				_, err = tx.ExecContext(ctx, string(migrationSQL))
				require.NoError(t, err)
				var actual string
				err = tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'SUBSCRIPTION_USD_TO_CNY_RATE'`).Scan(&actual)
				if tt.want == nil {
					require.ErrorIs(t, err, sql.ErrNoRows)
				} else {
					require.NoError(t, err)
					require.Equal(t, *tt.want, actual)
				}
			}
			if tt.legacy != nil {
				var legacy string
				require.NoError(t, tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'SUBSCRIPTION_CNY_PAYMENT_MULTIPLIER'`).Scan(&legacy))
				require.Equal(t, *tt.legacy, legacy)
			}
		})
	}
}
