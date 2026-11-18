//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGroupUsageSummaryRevalidationPreservesRetentionFloor(t *testing.T) {
	for _, tc := range []struct {
		name          string
		closedBefore  string
		timezone      string
		expiredBefore time.Time
		advanceFloor  bool
	}{
		{name: "retained date predates floor", closedBefore: "2026-08-14", timezone: "Asia/Shanghai", expiredBefore: time.Date(2026, 8, 11, 16, 0, 0, 0, time.UTC)},
		{name: "closed date predates floor", closedBefore: "2026-08-11", timezone: "Asia/Shanghai", expiredBefore: time.Date(2026, 8, 11, 16, 0, 0, 0, time.UTC)},
		{name: "invalid watermark", closedBefore: "2026-08-14", timezone: "UTC", expiredBefore: time.Date(2026, 8, 11, 16, 0, 0, 0, time.UTC)},
		{name: "floor advances between queries", closedBefore: "2026-08-14", timezone: "Asia/Shanghai", expiredBefore: time.Date(2026, 8, 10, 16, 0, 0, 0, time.UTC), advanceFloor: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			useGroupUsageRepositoryTestTimezone(t, "Asia/Shanghai")
			schema := createGroupUsageRollupTriggerTestSchema(t, ctx, false)
			db := openGroupUsageRollupSchemaDB(t, schema, "Asia/Shanghai")
			execGroupUsageRollupSQL(t, ctx, db, `
				INSERT INTO groups (id) VALUES (10);
				INSERT INTO users (id) VALUES (1);
				INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at) VALUES
					(1, 1, 10, 100, TIMESTAMPTZ '2026-08-11 12:00:00+08'),
					(2, 1, 10, 3, TIMESTAMPTZ '2026-08-13 12:00:00+08'),
					(3, 1, 10, 4, TIMESTAMPTZ '2026-08-14 12:00:00+08');
				INSERT INTO usage_group_daily_rollups (bucket_date, group_id, actual_cost) VALUES
					(DATE '2026-08-11', 10, 100),
					(DATE '2026-08-13', 10, 3);
			`)
			_, err := db.ExecContext(ctx, `
				UPDATE usage_group_rollup_state
				SET closed_before = $1::date,
					retained_from = TIMESTAMPTZ '2026-08-11 00:00:00+08',
					timezone_name = $2,
					expired_before = $3
				WHERE id = 1
			`, tc.closedBefore, tc.timezone, tc.expiredBefore)
			require.NoError(t, err)
			executor := groupUsageSummaryConcurrentWrite{sqlExecutor: db}
			if tc.advanceFloor {
				executor.beforeSummary = func() {
					// Retention is published before physical rows are removed.
					execGroupUsageRollupSQL(t, ctx, db, `
						UPDATE usage_group_rollup_state
						SET expired_before = TIMESTAMPTZ '2026-08-12 00:00:00+08'
						WHERE id = 1
					`)
				}
			}
			repo := newUsageLogRepositoryWithSQL(nil, executor)
			result, err := repo.GetAllGroupUsageSummary(ctx, time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC))

			require.NoError(t, err)
			require.Len(t, result, 1)
			require.InDelta(t, 7, result[0].TotalCost, 1e-9, "retention-expired rows must not return through the raw fallback")
			require.InDelta(t, 4, result[0].TodayCost, 1e-9)
			require.InDelta(t, 3, result[0].YesterdayCost, 1e-9)
		})
	}
}
