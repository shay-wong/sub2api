package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func newUsageLogProjectScopeSQLMock(t *testing.T, projectID int64, requiredFragments ...string) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	matcher := sqlmock.QueryMatcherFunc(func(_, actualSQL string) error {
		if needle := fmt.Sprintf("pp.project_id = %d", projectID); !strings.Contains(actualSQL, needle) {
			return fmt.Errorf("missing project scope %q in query: %s", needle, actualSQL)
		}
		for _, fragment := range requiredFragments {
			if !strings.Contains(actualSQL, fragment) {
				return fmt.Errorf("missing project scope fragment %q in query: %s", fragment, actualSQL)
			}
		}
		return nil
	})
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, mock
}

func TestUsageLogTrendQueriesPreserveProjectProfileScope(t *testing.T) {
	const projectID int64 = 7
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	ctx := service.WithProjectID(t.Context(), projectID)

	t.Run("api key trend scopes ranking and result rows", func(t *testing.T) {
		db, mock := newUsageLogProjectScopeSQLMock(t, projectID, "pm.user_id = usage_logs.user_id", "pm.user_id = u.user_id")
		repo := &usageLogRepository{sql: db}
		mock.ExpectQuery("project-scoped").
			WithArgs(start, end, 12, start, end).
			WillReturnRows(sqlmock.NewRows([]string{"date", "api_key_id", "key_name", "requests", "tokens"}))

		_, err := repo.GetAPIKeyUsageTrend(ctx, start, end, "day", 12)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("user trend scopes ranking and result rows", func(t *testing.T) {
		db, mock := newUsageLogProjectScopeSQLMock(t, projectID, "pm.user_id = usage_logs.user_id", "pm.user_id = u.user_id")
		repo := &usageLogRepository{sql: db}
		mock.ExpectQuery("project-scoped").
			WithArgs(start, end, 12, start, end).
			WillReturnRows(sqlmock.NewRows([]string{"date", "user_id", "email", "username", "requests", "tokens", "cost", "actual_cost"}))

		_, err := repo.GetUserUsageTrend(ctx, start, end, "day", 12)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("user spending ranking is project scoped", func(t *testing.T) {
		db, mock := newUsageLogProjectScopeSQLMock(t, projectID, "pm.user_id = u.user_id")
		repo := &usageLogRepository{sql: db}
		mock.ExpectQuery("project-scoped").
			WithArgs(start, end, 12).
			WillReturnRows(sqlmock.NewRows([]string{"user_id", "email", "actual_cost", "requests", "tokens", "total_actual_cost", "total_requests", "total_tokens"}))

		_, err := repo.GetUserSpendingRanking(ctx, start, end, 12)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("user trend by id is project scoped", func(t *testing.T) {
		db, mock := newUsageLogProjectScopeSQLMock(t, projectID, "pm.user_id = usage_logs.user_id")
		repo := &usageLogRepository{sql: db}
		mock.ExpectQuery("project-scoped").
			WithArgs(int64(42), start, end).
			WillReturnRows(sqlmock.NewRows([]string{"date", "requests", "input_tokens", "output_tokens", "cache_creation_tokens", "cache_read_tokens", "total_tokens", "cost", "actual_cost"}))

		_, err := repo.GetUserUsageTrendByUserID(ctx, 42, start, end, "day")
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("user breakdown is project scoped", func(t *testing.T) {
		db, mock := newUsageLogProjectScopeSQLMock(t, projectID, "pm.user_id = ul.user_id")
		repo := &usageLogRepository{sql: db}
		mock.ExpectQuery("project-scoped").
			WithArgs(start, end).
			WillReturnRows(sqlmock.NewRows([]string{"user_id", "email", "requests", "input_tokens", "output_tokens", "cache_tokens", "total_tokens", "cost", "actual_cost", "account_cost"}))

		_, err := repo.GetUserBreakdownStats(ctx, start, end, usagestats.UserBreakdownDimension{}, 12)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("group summary scopes groups and preserves empty left joins", func(t *testing.T) {
		useGroupUsageRepositoryTestTimezone(t, "UTC")
		db, mock := newUsageLogProjectScopeSQLMock(t, projectID, "LEFT JOIN usage_logs ul ON ul.group_id = g.id AND", "ul.created_at >= $2 AND ul.created_at < $1", "ppb.resource_id = g.id", "pm.user_id = ul.user_id")
		repo := &usageLogRepository{sql: db}
		mock.ExpectQuery("project-scoped").
			WithArgs(start, start.AddDate(0, 0, -1)).
			WillReturnRows(sqlmock.NewRows([]string{"group_id", "total_cost", "today_cost", "yesterday_cost"}))

		_, err := repo.GetAllGroupUsageSummary(ctx, start)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
