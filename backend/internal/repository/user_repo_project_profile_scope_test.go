package repository

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestUserRepositoryBatchSetConcurrencyUsesProjectProfileScope(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec("UPDATE users SET concurrency = \\$1, updated_at = NOW\\(\\) WHERE id = ANY\\(\\$2\\) AND deleted_at IS NULL.*pp.mode = 'unrestricted'.*project_profile_bindings.*pp.mode = 'restricted'").
		WithArgs(3, pq.Array([]int64{10, 11})).
		WillReturnResult(sqlmock.NewResult(0, 1))

	repo := newUserRepositoryWithSQL(nil, db)
	affected, err := repo.BatchSetConcurrency(service.WithProjectID(context.Background(), 101), []int64{10, 11}, 3)

	require.NoError(t, err)
	require.Equal(t, 1, affected)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepositoryBatchAddConcurrencyUsesProjectProfileScope(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec("UPDATE users SET concurrency = GREATEST\\(concurrency \\+ \\$1, 0\\), updated_at = NOW\\(\\) WHERE id = ANY\\(\\$2\\) AND deleted_at IS NULL.*pp.mode = 'unrestricted'.*project_profile_bindings.*pp.mode = 'restricted'").
		WithArgs(2, pq.Array([]int64{10, 11})).
		WillReturnResult(sqlmock.NewResult(0, 2))

	repo := newUserRepositoryWithSQL(nil, db)
	affected, err := repo.BatchAddConcurrency(service.WithProjectID(context.Background(), 101), []int64{10, 11}, 2)

	require.NoError(t, err)
	require.Equal(t, 2, affected)
	require.NoError(t, mock.ExpectationsWereMet())
}
