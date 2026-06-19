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

	mock.ExpectExec("UPDATE users SET concurrency = \\$1, updated_at = NOW\\(\\) WHERE id = ANY\\(\\$2\\) AND deleted_at IS NULL.*FROM project_members pm.*pm.project_id = 101.*pm.user_id = users.id").
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

	mock.ExpectExec("UPDATE users SET concurrency = GREATEST\\(concurrency \\+ \\$1, 0\\), updated_at = NOW\\(\\) WHERE id = ANY\\(\\$2\\) AND deleted_at IS NULL.*FROM project_members pm.*pm.project_id = 101.*pm.user_id = users.id").
		WithArgs(2, pq.Array([]int64{10, 11})).
		WillReturnResult(sqlmock.NewResult(0, 2))

	repo := newUserRepositoryWithSQL(nil, db)
	affected, err := repo.BatchAddConcurrency(service.WithProjectID(context.Background(), 101), []int64{10, 11}, 2)

	require.NoError(t, err)
	require.Equal(t, 2, affected)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepositoryLoadProjectRolesPopulatesScopedUsersIncludingDisabledMembers(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery("SELECT user_id, role, status, COALESCE\\(scopes, '\\[\\]'::jsonb\\) FROM project_members WHERE project_id = \\$1 AND user_id = ANY\\(\\$2\\)").
		WithArgs(int64(101), pq.Array([]int64{10, 11})).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role", "status", "scopes"}).
			AddRow(int64(10), service.ProjectRoleAdmin, service.StatusActive, `["`+service.AdminPermissionDashboardRead+`"]`).
			AddRow(int64(11), service.ProjectRoleUser, service.StatusDisabled, `[]`))

	repo := newUserRepositoryWithSQL(nil, db)
	users := map[int64]*service.User{
		10: {ID: 10, Role: service.RoleUser},
		11: {ID: 11, Role: service.RoleUser},
	}

	err = repo.loadProjectRoles(service.WithProjectID(context.Background(), 101), []int64{10, 11}, users)

	require.NoError(t, err)
	require.Equal(t, service.ProjectRoleAdmin, users[10].ProjectRole)
	require.Equal(t, service.StatusActive, users[10].ProjectMemberStatus)
	require.Equal(t, []string{service.AdminPermissionDashboardRead}, users[10].ProjectPermissions)
	require.Equal(t, service.ProjectRoleUser, users[11].ProjectRole)
	require.Equal(t, service.StatusDisabled, users[11].ProjectMemberStatus)
	require.NoError(t, mock.ExpectationsWereMet())
}
