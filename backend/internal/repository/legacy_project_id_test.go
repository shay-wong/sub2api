//go:build unit

package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestEnsureDefaultProjectReusesExistingRowWithoutWriting(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM projects WHERE slug = $1`)).
		WithArgs(defaultProjectSlug).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(42)))

	projectID, err := ensureDefaultProject(context.Background(), db)

	require.NoError(t, err)
	require.Equal(t, int64(42), projectID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnsureDefaultProjectCreatesMissingRowThenReadsIt(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	lookup := regexp.QuoteMeta(`SELECT id FROM projects WHERE slug = $1`)
	mock.ExpectQuery(lookup).
		WithArgs(defaultProjectSlug).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(`INSERT INTO projects[\s\S]+ON CONFLICT \(slug\) DO NOTHING`).
		WithArgs(defaultProjectName, defaultProjectSlug, "Legacy default project for required project_id columns.", "{}").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(lookup).
		WithArgs(defaultProjectSlug).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(43)))

	projectID, err := ensureDefaultProject(context.Background(), db)

	require.NoError(t, err)
	require.Equal(t, int64(43), projectID)
	require.NoError(t, mock.ExpectationsWereMet())
}
