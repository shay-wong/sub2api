package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestUserGroupRateLimitWindowRepositoryListByGroup(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &userGroupRateLimitWindowRepository{sql: db}
	ctx := context.Background()
	windowStart := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`(?s)SELECT COUNT\(\*\).*FROM user_group_rate_limit_windows`).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(3)))
	mock.ExpectQuery(`(?s)SELECT w\.user_id, w\.group_id, g\.name, g\.rate_limit_5h, w\.usage_5h_usd, w\.window_5h_start.*ORDER BY w\.usage_5h_usd DESC, w\.user_id ASC`).
		WithArgs(int64(3), 2, 2).
		WillReturnRows(sqlmock.NewRows([]string{
			"user_id",
			"group_id",
			"name",
			"rate_limit_5h",
			"usage_5h_usd",
			"window_5h_start",
		}).
			AddRow(int64(7), int64(3), "pro", 25.0, 12.5, windowStart))

	got, page, err := repo.ListByGroup(ctx, 3, pagination.PaginationParams{Page: 2, PageSize: 2})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, int64(7), got[0].UserID)
	require.Equal(t, int64(3), got[0].GroupID)
	require.Equal(t, "pro", got[0].GroupName)
	require.InDelta(t, 25.0, got[0].RateLimit5h, 1e-12)
	require.InDelta(t, 12.5, got[0].Usage5hUSD, 1e-12)
	require.NotNil(t, got[0].Window5hStart)
	require.Equal(t, windowStart, *got[0].Window5hStart)
	require.NotNil(t, page)
	require.Equal(t, int64(3), page.Total)
	require.Equal(t, 2, page.Page)
	require.Equal(t, 2, page.PageSize)
	require.Equal(t, 2, page.Pages)
	require.NoError(t, mock.ExpectationsWereMet())
}
