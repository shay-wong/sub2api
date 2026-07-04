package repository

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupAccountRelationshipScopeUsesActiveProjectProfile(t *testing.T) {
	clause := groupAccountRelationshipScopeClause(service.WithProjectID(context.Background(), 7), "ag.group_id", "a.id")
	normalized := normalizeSQLWhitespace(clause)

	require.NotContains(t, normalized, "a.project_id =")
	require.Contains(t, normalized, "pp.mode = 'unrestricted'")
	require.Contains(t, normalized, "FROM project_profiles pp")
	require.Contains(t, normalized, "JOIN project_profile_bindings ppb")
	require.Contains(t, normalized, "ppb.resource_type = 'group'")
	require.Contains(t, normalized, "ppb.resource_id = ag.group_id")
	require.Contains(t, normalized, "ppb.resource_type = 'account'")
	require.Contains(t, normalized, "ppb.resource_id = a.id")
}

func TestGroupOnlyScopeUsesActiveProjectProfile(t *testing.T) {
	clause := groupOnlyScopeClause(service.WithProjectID(context.Background(), 7))
	normalized := normalizeSQLWhitespace(clause)

	require.NotContains(t, normalized, "groups.project_id =")
	require.NotContains(t, normalized, "group.project_id =")
	require.Contains(t, normalized, "pp.mode = 'unrestricted'")
	require.Contains(t, normalized, "ppb.resource_type = 'group'")
	require.Contains(t, normalized, "ppb.resource_id = groups.id")
}

func TestGroupRepositoryListActiveIDsSQLUsesActiveProjectProfileScope(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	var capturedSQL string
	mock.ExpectQuery("SELECT id").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	repo := newGroupRepositoryWithSQL(nil, captureQuerySQL{db: db, captured: &capturedSQL})

	ids, err := repo.ListActiveIDs(service.WithProjectID(context.Background(), 7))
	require.NoError(t, err)
	require.Empty(t, ids)

	normalized := normalizeSQLWhitespace(capturedSQL)
	require.Contains(t, normalized, "FROM project_profiles pp")
	require.Contains(t, normalized, "JOIN project_profile_bindings ppb")
	require.Contains(t, normalized, "pp.mode = 'unrestricted'")
	require.Contains(t, normalized, "ppb.resource_type = 'group'")
	require.Contains(t, normalized, "ppb.resource_id = groups.id")
	require.NoError(t, mock.ExpectationsWereMet())
}
