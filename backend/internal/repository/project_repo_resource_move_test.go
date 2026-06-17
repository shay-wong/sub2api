package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestProjectRepositoryMoveProjectResourcesMovesResourcesAndCleansCrossProjectLinks(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewProjectRepository(db)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT DISTINCT key").
		WillReturnRows(sqlmock.NewRows([]string{"key"}).AddRow("sk-project-key"))
	mock.ExpectQuery("SELECT DISTINCT user_id\\s+FROM \\(").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(int64(7)))
	mock.ExpectQuery("SELECT DISTINCT user_id\\s+FROM api_keys").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(int64(8)))
	mock.ExpectExec("UPDATE groups").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE accounts").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("UPDATE api_keys").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("SET fallback_group_id = NULL").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("SET fallback_group_id_on_invalid_request = NULL").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("WITH filtered AS").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM account_groups").
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec("SET group_id = NULL").
		WillReturnResult(sqlmock.NewResult(0, 4))
	mock.ExpectExec("INSERT INTO project_members").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("UPDATE usage_logs").
		WillReturnResult(sqlmock.NewResult(0, 5))
	mock.ExpectExec("UPDATE ops_error_logs").
		WillReturnResult(sqlmock.NewResult(0, 6))
	mock.ExpectExec("INSERT INTO scheduler_outbox").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := repo.MoveProjectResources(context.Background(), 99, service.ProjectResourceMoveInput{
		AccountIDs:       []int64{11, 12},
		APIKeyIDs:        []int64{21},
		GroupIDs:         []int64{31},
		MoveUsageHistory: true,
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), result.AccountsMoved)
	require.Equal(t, int64(1), result.APIKeysMoved)
	require.Equal(t, int64(1), result.GroupsMoved)
	require.Equal(t, int64(3), result.AccountGroupBindingsRemoved)
	require.Equal(t, int64(4), result.APIKeyGroupBindingsCleared)
	require.Equal(t, int64(1), result.GroupFallbacksCleared)
	require.Equal(t, int64(1), result.GroupModelRoutingCleared)
	require.Equal(t, int64(2), result.ProjectMembersAdded)
	require.Equal(t, int64(5), result.UsageLogsMoved)
	require.Equal(t, int64(6), result.OpsErrorLogsMoved)
	require.Equal(t, []string{"sk-project-key"}, result.InvalidatedAPIKeys)
	require.Equal(t, []int64{7, 8}, result.InvalidatedUserIDs)
	require.Equal(t, []int64{31}, result.InvalidatedGroupIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}
