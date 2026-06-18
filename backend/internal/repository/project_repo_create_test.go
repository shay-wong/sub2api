package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestProjectRepositoryCreateProjectSkipsBindingsForUnrestrictedDefaultProfile(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO projects (name, slug, description, status, profiles, created_at, updated_at)
		VALUES ($1, $2, $3, 'active', '{}'::jsonb, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING id, name, slug, description
	`)).
		WithArgs("Ops Space", "ops-space", nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slug", "description"}).
			AddRow(int64(101), "Ops Space", "ops-space", nil))
	mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO project_members (project_id, user_id, role, scopes, is_owner, created_at, updated_at)
		VALUES ($1, $2, $3, '[]', TRUE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (project_id, user_id) DO UPDATE
		SET role = EXCLUDED.role,
			is_owner = TRUE,
			updated_at = CURRENT_TIMESTAMP
	`)).
		WithArgs(int64(101), int64(10), service.ProjectRoleAdmin).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO project_profiles (project_id, name, description, mode, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, TRUE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT DO NOTHING
		RETURNING id
	`)).
		WithArgs(int64(101), "默认配置", "Default active project application profile.", service.ProjectProfileModeUnrestricted).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(202)))
	mock.ExpectCommit()

	repo := NewProjectRepository(db)
	project, err := repo.CreateProject(context.Background(), service.ProjectCreateInput{
		Name:        "Ops Space",
		Slug:        "ops-space",
		OwnerUserID: 10,
		ProfileMode: service.ProjectProfileModeUnrestricted,
		Bindings: service.ProjectProfileBindingInput{
			UserIDs:         []int64{10, 11},
			GroupIDs:        []int64{20},
			AccountIDs:      []int64{30},
			SubscriptionIDs: []int64{40},
			APIKeyIDs:       []int64{50},
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(101), project.ID)
	require.Equal(t, "Ops Space", project.Name)
	require.Equal(t, "ops-space", project.Slug)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProjectRepositoryCreateProjectCreatesRestrictedDefaultProfileBindings(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	expectCreateProjectBase(t, mock, service.ProjectProfileModeRestricted)
	for _, item := range []struct {
		resourceType string
		ids          []int64
	}{
		{service.ProjectResourceTypeUser, []int64{10, 11}},
		{service.ProjectResourceTypeGroup, []int64{20}},
		{service.ProjectResourceTypeAccount, []int64{30}},
		{service.ProjectResourceTypeSubscription, []int64{40}},
		{service.ProjectResourceTypeAPIKey, []int64{50}},
	} {
		mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO project_profile_bindings (project_profile_id, resource_type, resource_id, created_at)
		SELECT $1, $2, unnest($3::bigint[]), CURRENT_TIMESTAMP
		ON CONFLICT (project_profile_id, resource_type, resource_id) DO NOTHING
	`)).
			WithArgs(int64(202), item.resourceType, pq.Array(item.ids)).
			WillReturnResult(sqlmock.NewResult(0, int64(len(item.ids))))
	}
	mock.ExpectCommit()

	repo := NewProjectRepository(db)
	project, err := repo.CreateProject(context.Background(), service.ProjectCreateInput{
		Name:        "Ops Space",
		Slug:        "ops-space",
		OwnerUserID: 10,
		ProfileMode: service.ProjectProfileModeRestricted,
		Bindings: service.ProjectProfileBindingInput{
			UserIDs:         []int64{10, 11},
			GroupIDs:        []int64{20},
			AccountIDs:      []int64{30},
			SubscriptionIDs: []int64{40},
			APIKeyIDs:       []int64{50},
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(101), project.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProjectRepositoryListUserProjectsOnlyReturnsActiveMemberships(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT p.id, p.name, p.slug, p.description, pm.role, pm.is_owner
		FROM project_members pm
		JOIN projects p ON p.id = pm.project_id
		WHERE pm.user_id = $1
		  AND pm.status = 'active'
		  AND p.deleted_at IS NULL
		  AND p.status = 'active'
		ORDER BY pm.is_owner DESC, p.id ASC
	`)).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slug", "description", "role", "is_owner"}).
			AddRow(int64(101), "Ops Space", "ops-space", nil, service.ProjectRoleAdmin, true))

	repo := NewProjectRepository(db)
	projects, err := repo.ListUserProjects(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, projects, 1)
	require.Equal(t, int64(101), projects[0].ID)
	require.Equal(t, service.ProjectRoleAdmin, projects[0].Role)
	require.True(t, projects[0].IsOwner)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProjectRepositorySetActiveMemberBindsUserToActiveProfile(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id FROM users WHERE id = $1 AND deleted_at IS NULL LIMIT 1
	`)).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(10)))
	mock.ExpectQuery(regexp.QuoteMeta(`
		WITH upserted AS (
			INSERT INTO project_members (project_id, user_id, role, scopes, is_owner, status, created_at, updated_at)
			VALUES ($1, $2, $3, '[]', $4, $5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			ON CONFLICT (project_id, user_id) DO UPDATE
			SET role = EXCLUDED.role,
				is_owner = EXCLUDED.is_owner,
				status = EXCLUDED.status,
				updated_at = CURRENT_TIMESTAMP
			RETURNING project_id, user_id, role, is_owner, status, created_at, updated_at
		)
		SELECT
			up.project_id,
			up.user_id,
			u.email,
			COALESCE(u.username, ''),
			up.role,
			up.is_owner,
			up.status,
			u.status,
			up.created_at,
			up.updated_at
		FROM upserted up
		JOIN users u ON u.id = up.user_id
	`)).
		WithArgs(int64(101), int64(10), service.ProjectRoleUser, false, service.StatusActive).
		WillReturnRows(sqlmock.NewRows([]string{
			"project_id", "user_id", "email", "username", "role", "is_owner", "status", "user_status", "created_at", "updated_at",
		}).AddRow(int64(101), int64(10), "member@example.com", "member", service.ProjectRoleUser, false, service.StatusActive, service.StatusActive, now, now))
	expectBindActiveProfileResource(mock, int64(101), service.ProjectResourceTypeUser, int64(10))
	mock.ExpectCommit()

	repo := NewProjectRepository(db)
	member, err := repo.SetProjectMember(context.Background(), 101, service.ProjectMemberInput{
		UserID: 10,
		Role:   service.ProjectRoleUser,
	})
	require.NoError(t, err)
	require.Equal(t, int64(10), member.UserID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProjectRepositoryDisableMemberRemovesUserProfileBindings(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	status := service.StatusDisabled
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id FROM users WHERE id = $1 AND deleted_at IS NULL LIMIT 1
	`)).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(10)))
	mock.ExpectQuery(regexp.QuoteMeta(`
		WITH upserted AS (
			INSERT INTO project_members (project_id, user_id, role, scopes, is_owner, status, created_at, updated_at)
			VALUES ($1, $2, $3, '[]', $4, $5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			ON CONFLICT (project_id, user_id) DO UPDATE
			SET role = EXCLUDED.role,
				is_owner = EXCLUDED.is_owner,
				status = EXCLUDED.status,
				updated_at = CURRENT_TIMESTAMP
			RETURNING project_id, user_id, role, is_owner, status, created_at, updated_at
		)
		SELECT
			up.project_id,
			up.user_id,
			u.email,
			COALESCE(u.username, ''),
			up.role,
			up.is_owner,
			up.status,
			u.status,
			up.created_at,
			up.updated_at
		FROM upserted up
		JOIN users u ON u.id = up.user_id
	`)).
		WithArgs(int64(101), int64(10), service.ProjectRoleUser, false, service.StatusDisabled).
		WillReturnRows(sqlmock.NewRows([]string{
			"project_id", "user_id", "email", "username", "role", "is_owner", "status", "user_status", "created_at", "updated_at",
		}).AddRow(int64(101), int64(10), "member@example.com", "member", service.ProjectRoleUser, false, service.StatusDisabled, service.StatusActive, now, now))
	expectRemoveProjectProfileResourceBindings(mock, int64(101), service.ProjectResourceTypeUser, int64(10))
	mock.ExpectCommit()

	repo := NewProjectRepository(db)
	member, err := repo.SetProjectMember(context.Background(), 101, service.ProjectMemberInput{
		UserID: 10,
		Role:   service.ProjectRoleUser,
		Status: &status,
	})
	require.NoError(t, err)
	require.Equal(t, service.StatusDisabled, member.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProjectRepositoryRemoveMemberRemovesUserProfileBindings(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT is_owner
		FROM project_members
		WHERE project_id = $1
		  AND user_id = $2
	`)).
		WithArgs(int64(101), int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"is_owner"}).AddRow(false))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`
		DELETE FROM project_members
		WHERE project_id = $1
		  AND user_id = $2
	`)).
		WithArgs(int64(101), int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectRemoveProjectProfileResourceBindings(mock, int64(101), service.ProjectResourceTypeUser, int64(10))
	mock.ExpectCommit()

	repo := NewProjectRepository(db)
	require.NoError(t, repo.RemoveProjectMember(context.Background(), 101, 10))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProjectRepositoryUpdateProjectProfileClearsBindingsForUnrestrictedMode(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	mode := service.ProjectProfileModeUnrestricted
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`
		UPDATE project_profiles
		SET name = COALESCE($3, name),
			description = COALESCE($4, description),
			mode = COALESCE($5, mode),
			updated_at = CURRENT_TIMESTAMP
		WHERE project_id = $1
		  AND id = $2
		  AND deleted_at IS NULL
		RETURNING id, project_id, name, description, mode, is_active, created_at, updated_at
	`)).
		WithArgs(int64(101), int64(202), nil, nil, mode).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "name", "description", "mode", "is_active", "created_at", "updated_at"}).
			AddRow(int64(202), int64(101), "Default", nil, mode, true, now, now))
	mock.ExpectExec(regexp.QuoteMeta(`
			DELETE FROM project_profile_bindings
			WHERE project_profile_id = $1
		`)).
		WithArgs(int64(202)).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()

	repo := NewProjectRepository(db)
	profile, err := repo.UpdateProjectProfile(context.Background(), 101, 202, service.ProjectProfileInput{Mode: &mode})
	require.NoError(t, err)
	require.Equal(t, service.ProjectProfileModeUnrestricted, profile.Mode)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProjectRepositorySetBindingsClearsAndIgnoresBindingsForUnrestrictedProfile(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT mode
		FROM project_profiles
		WHERE project_id = $1
		  AND id = $2
		  AND deleted_at IS NULL
	`)).
		WithArgs(int64(101), int64(202)).
		WillReturnRows(sqlmock.NewRows([]string{"mode"}).AddRow(service.ProjectProfileModeUnrestricted))
	mock.ExpectExec(regexp.QuoteMeta(`
			DELETE FROM project_profile_bindings
			WHERE project_profile_id = $1
		`)).
		WithArgs(int64(202)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta(`
			UPDATE project_profiles
			SET updated_at = CURRENT_TIMESTAMP
			WHERE id = $1
		`)).
		WithArgs(int64(202)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	repo := NewProjectRepository(db)
	bindings, err := repo.SetProjectProfileBindings(context.Background(), 101, 202, service.ProjectProfileBindingInput{
		UserIDs: []int64{10},
	})
	require.NoError(t, err)
	require.Equal(t, int64(202), bindings.ProfileID)
	require.Empty(t, bindings.UserIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProjectRepositoryValidateProjectProfileBindingScopeRejectsInvisibleResource(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM users").
		WithArgs(pq.Array([]int64{99})).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	repo := NewProjectRepository(db)
	err = repo.ValidateProjectProfileBindingScope(context.Background(), 101, service.ProjectProfileBindingInput{
		UserIDs: []int64{99},
	})
	require.ErrorIs(t, err, service.ErrProjectAccessForbidden)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProjectRepositoryValidateProjectProfileBindingResourcesRejectsMissingResource(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM users WHERE id = ANY\\(\\$1\\) AND deleted_at IS NULL").
		WithArgs(pq.Array([]int64{99})).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	repo := NewProjectRepository(db)
	err = repo.ValidateProjectProfileBindingResources(context.Background(), service.ProjectProfileBindingInput{
		UserIDs: []int64{99},
	})
	require.ErrorIs(t, err, service.ErrProjectInvalidInput)
	require.NoError(t, mock.ExpectationsWereMet())
}

func expectBindActiveProfileResource(mock sqlmock.Sqlmock, projectID int64, resourceType string, resourceID int64) {
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id
		FROM project_profiles
		WHERE project_id = $1
		  AND is_active = TRUE
		  AND deleted_at IS NULL
		LIMIT 1
	`)).
		WithArgs(projectID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(202)))
	mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO project_profile_bindings (project_profile_id, resource_type, resource_id, created_at, metadata)
		SELECT pp.id, $2, $3, CURRENT_TIMESTAMP, $5
		FROM project_profiles pp
		WHERE pp.project_id = $1
		  AND pp.is_active = TRUE
		  AND pp.deleted_at IS NULL
		  AND pp.mode = $4
		ON CONFLICT (project_profile_id, resource_type, resource_id) DO NOTHING
	`)).
		WithArgs(projectID, resourceType, resourceID, service.ProjectProfileModeRestricted, "{}").
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectRemoveProjectProfileResourceBindings(mock sqlmock.Sqlmock, projectID int64, resourceType string, resourceID int64) {
	mock.ExpectExec(regexp.QuoteMeta(`
		DELETE FROM project_profile_bindings ppb
		USING project_profiles pp
		WHERE ppb.project_profile_id = pp.id
		  AND pp.project_id = $1
		  AND ppb.resource_type = $2
		  AND ppb.resource_id = $3
	`)).
		WithArgs(projectID, resourceType, resourceID).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectCreateProjectBase(t *testing.T, mock sqlmock.Sqlmock, profileMode string) {
	t.Helper()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO projects (name, slug, description, status, profiles, created_at, updated_at)
		VALUES ($1, $2, $3, 'active', '{}'::jsonb, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING id, name, slug, description
	`)).
		WithArgs("Ops Space", "ops-space", nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slug", "description"}).
			AddRow(int64(101), "Ops Space", "ops-space", nil))
	mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO project_members (project_id, user_id, role, scopes, is_owner, created_at, updated_at)
		VALUES ($1, $2, $3, '[]', TRUE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (project_id, user_id) DO UPDATE
		SET role = EXCLUDED.role,
			is_owner = TRUE,
			updated_at = CURRENT_TIMESTAMP
	`)).
		WithArgs(int64(101), int64(10), service.ProjectRoleAdmin).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO project_profiles (project_id, name, description, mode, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, TRUE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT DO NOTHING
		RETURNING id
	`)).
		WithArgs(int64(101), "默认配置", "Default active project application profile.", profileMode).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(202)))
}
