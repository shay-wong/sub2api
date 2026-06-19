package repository

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserGroupRateRepository_GetByGroupIDUsesProjectProfileUserScope(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	var capturedSQL string
	repo := &userGroupRateRepository{sql: captureQuerySQL{db: db, captured: &capturedSQL}}
	mock.ExpectQuery("SELECT ugr.user_id").
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{
			"user_id", "username", "email", "notes", "status", "rate_multiplier", "rpm_override",
		}))

	entries, err := repo.GetByGroupID(service.WithProjectID(context.Background(), 7), 42)

	require.NoError(t, err)
	require.Empty(t, entries)
	normalized := normalizeSQLWhitespace(capturedSQL)
	require.Contains(t, normalized, "pp.mode = 'unrestricted'")
	require.Contains(t, normalized, "FROM project_profiles pp")
	require.Contains(t, normalized, "JOIN project_profile_bindings ppb")
	require.Contains(t, normalized, "ppb.resource_type = 'group'")
	require.Contains(t, normalized, "ppb.resource_id = ugr.group_id")
	require.Contains(t, normalized, "FROM project_members pm")
	require.Contains(t, normalized, "pm.user_id = ugr.user_id")
	require.NotContains(t, normalized, "ppb.resource_type = 'user'")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserGroupRateRepository_GetByUserIDUsesProjectProfileGroupAndUserScope(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	var capturedSQL string
	repo := &userGroupRateRepository{sql: captureQuerySQL{db: db, captured: &capturedSQL}}
	mock.ExpectQuery("SELECT group_id").
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "rate_multiplier"}))

	rates, err := repo.GetByUserID(service.WithProjectID(context.Background(), 7), 42)

	require.NoError(t, err)
	require.Empty(t, rates)
	normalized := normalizeSQLWhitespace(capturedSQL)
	require.Contains(t, normalized, "pp.mode = 'unrestricted'")
	require.Contains(t, normalized, "FROM project_profiles pp")
	require.Contains(t, normalized, "JOIN project_profile_bindings ppb")
	require.Contains(t, normalized, "ppb.resource_type = 'group'")
	require.Contains(t, normalized, "ppb.resource_id = user_group_rate_multipliers.group_id")
	require.Contains(t, normalized, "FROM project_members pm")
	require.Contains(t, normalized, "pm.user_id = user_group_rate_multipliers.user_id")
	require.NotContains(t, normalized, "ppb.resource_type = 'user'")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserGroupRateRepository_GetByUserIDsUsesProjectProfileGroupAndUserScope(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	var capturedSQL string
	repo := &userGroupRateRepository{sql: captureQuerySQL{db: db, captured: &capturedSQL}}
	mock.ExpectQuery("SELECT user_id").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "group_id", "rate_multiplier"}))

	rates, err := repo.GetByUserIDs(service.WithProjectID(context.Background(), 7), []int64{42, 42, 51})

	require.NoError(t, err)
	require.Len(t, rates, 2)
	normalized := normalizeSQLWhitespace(capturedSQL)
	require.Contains(t, normalized, "pp.mode = 'unrestricted'")
	require.Contains(t, normalized, "FROM project_profiles pp")
	require.Contains(t, normalized, "JOIN project_profile_bindings ppb")
	require.Contains(t, normalized, "ppb.resource_type = 'group'")
	require.Contains(t, normalized, "ppb.resource_id = user_group_rate_multipliers.group_id")
	require.Contains(t, normalized, "FROM project_members pm")
	require.Contains(t, normalized, "pm.user_id = user_group_rate_multipliers.user_id")
	require.NotContains(t, normalized, "ppb.resource_type = 'user'")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserGroupRateRepository_SyncGroupRateMultipliersScopesProjectUserCleanup(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)}
	repo := &userGroupRateRepository{sql: exec}

	err := repo.SyncGroupRateMultipliers(service.WithProjectID(context.Background(), 7), 42, []service.GroupRateMultiplierInput{
		{UserID: 10, RateMultiplier: 1.5},
	})

	require.NoError(t, err)
	require.Len(t, exec.execQueries, 3)
	for _, query := range exec.execQueries {
		normalized := normalizeSQLWhitespace(query)
		require.Contains(t, normalized, "pp.mode = 'unrestricted'")
		require.Contains(t, normalized, "FROM project_profiles pp")
		require.Contains(t, normalized, "JOIN project_profile_bindings ppb")
		require.Contains(t, normalized, "ppb.resource_type = 'group'")
		require.Contains(t, normalized, "FROM project_members pm")
		require.NotContains(t, normalized, "ppb.resource_type = 'user'")
	}
	require.Contains(t, normalizeSQLWhitespace(exec.execQueries[0]), "ppb.resource_id = user_group_rate_multipliers.group_id")
	require.Contains(t, normalizeSQLWhitespace(exec.execQueries[1]), "ppb.resource_id = user_group_rate_multipliers.group_id")
	require.Contains(t, normalizeSQLWhitespace(exec.execQueries[2]), "ppb.resource_id = $1::bigint")
	require.Contains(t, normalizeSQLWhitespace(exec.execQueries[0]), "pm.user_id = user_group_rate_multipliers.user_id")
	require.Contains(t, normalizeSQLWhitespace(exec.execQueries[1]), "pm.user_id = user_group_rate_multipliers.user_id")
	require.Contains(t, normalizeSQLWhitespace(exec.execQueries[2]), "pm.user_id = data.user_id")
	require.Contains(t, normalizeSQLWhitespace(exec.execQueries[0]), "user_id <> ALL($2)")
}

func TestUserGroupRateRepository_SyncGroupRPMOverridesScopesProjectUserCleanup(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)}
	repo := &userGroupRateRepository{sql: exec}
	rpm := 120

	err := repo.SyncGroupRPMOverrides(service.WithProjectID(context.Background(), 7), 42, []service.GroupRPMOverrideInput{
		{UserID: 10, RPMOverride: &rpm},
	})

	require.NoError(t, err)
	require.Len(t, exec.execQueries, 3)
	for _, query := range exec.execQueries {
		normalized := normalizeSQLWhitespace(query)
		require.Contains(t, normalized, "pp.mode = 'unrestricted'")
		require.Contains(t, normalized, "FROM project_profiles pp")
		require.Contains(t, normalized, "JOIN project_profile_bindings ppb")
		require.Contains(t, normalized, "ppb.resource_type = 'group'")
		require.Contains(t, normalized, "FROM project_members pm")
		require.NotContains(t, normalized, "ppb.resource_type = 'user'")
	}
	require.Contains(t, normalizeSQLWhitespace(exec.execQueries[0]), "ppb.resource_id = user_group_rate_multipliers.group_id")
	require.Contains(t, normalizeSQLWhitespace(exec.execQueries[1]), "ppb.resource_id = user_group_rate_multipliers.group_id")
	require.Contains(t, normalizeSQLWhitespace(exec.execQueries[2]), "ppb.resource_id = $1::bigint")
	require.Contains(t, normalizeSQLWhitespace(exec.execQueries[0]), "pm.user_id = user_group_rate_multipliers.user_id")
	require.Contains(t, normalizeSQLWhitespace(exec.execQueries[1]), "pm.user_id = user_group_rate_multipliers.user_id")
	require.Contains(t, normalizeSQLWhitespace(exec.execQueries[2]), "pm.user_id = data.user_id")
	require.Contains(t, normalizeSQLWhitespace(exec.execQueries[0]), "user_id <> ALL($2)")
}

func TestUserGroupRateRepository_ClearGroupRPMOverridesScopesProjectUsers(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)}
	repo := &userGroupRateRepository{sql: exec}

	err := repo.ClearGroupRPMOverrides(service.WithProjectID(context.Background(), 7), 42)

	require.NoError(t, err)
	require.Len(t, exec.execQueries, 2)
	for _, query := range exec.execQueries {
		normalized := normalizeSQLWhitespace(query)
		require.Contains(t, normalized, "pp.mode = 'unrestricted'")
		require.Contains(t, normalized, "FROM project_profiles pp")
		require.Contains(t, normalized, "JOIN project_profile_bindings ppb")
		require.Contains(t, normalized, "ppb.resource_type = 'group'")
		require.Contains(t, normalized, "ppb.resource_id = user_group_rate_multipliers.group_id")
		require.Contains(t, normalized, "FROM project_members pm")
		require.Contains(t, normalized, "pm.user_id = user_group_rate_multipliers.user_id")
		require.NotContains(t, normalized, "ppb.resource_type = 'user'")
	}
}
