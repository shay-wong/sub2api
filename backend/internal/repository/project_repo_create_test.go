package repository

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func expectSchedulerFullRebuildOutbox(mock sqlmock.Sqlmock) {
	mock.ExpectExec(regexp.QuoteMeta(`
			INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload, dedup_key)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (dedup_key) WHERE dedup_key IS NOT NULL DO NOTHING
		`)).
		WithArgs(service.SchedulerOutboxEventFullRebuild, nil, nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestProjectRepositoryCreateProjectKeepsRestrictedDefaultBindingsWhenScopeIsUnrestricted(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	expectCreateProjectBase(t, mock, service.ProjectProfileModeRestricted)
	for _, item := range []struct {
		resourceType string
		ids          []int64
	}{
		{service.ProjectResourceTypeGroup, []int64{20}},
		{service.ProjectResourceTypeAccount, []int64{30}},
		{service.ProjectResourceTypeProxy, []int64{35}},
		{service.ProjectResourceTypeSubscription, []int64{40}},
	} {
		mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO project_profile_bindings (project_profile_id, resource_type, resource_id, created_at)
		SELECT $1, $2, unnest($3::bigint[]), CURRENT_TIMESTAMP
		ON CONFLICT (project_profile_id, resource_type, resource_id) DO NOTHING
	`)).
			WithArgs(int64(202), item.resourceType, pq.Array(item.ids)).
			WillReturnResult(sqlmock.NewResult(0, int64(len(item.ids))))
	}
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id
		FROM project_profiles
		WHERE project_id = $1
		  AND name = $2
		  AND mode = $3
		  AND deleted_at IS NULL
		ORDER BY id ASC
		LIMIT 1
	`)).
		WithArgs(int64(101), unrestrictedProjectScopeProfileName, service.ProjectProfileModeUnrestricted).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta(`
			INSERT INTO project_profiles (project_id, name, description, mode, is_active, created_at, updated_at)
			VALUES ($1, $2, $3, $4, FALSE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			RETURNING id
	`)).
		WithArgs(int64(101), unrestrictedProjectScopeProfileName, "Internal project-level unrestricted resource scope.", service.ProjectProfileModeUnrestricted).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(303)))
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE project_profiles
		SET is_active = FALSE,
			updated_at = CURRENT_TIMESTAMP
		WHERE project_id = $1
		  AND is_active = TRUE
		  AND deleted_at IS NULL
	`)).
		WithArgs(int64(101)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`
		UPDATE project_profiles
		SET is_active = TRUE,
			updated_at = CURRENT_TIMESTAMP
		WHERE project_id = $1
		  AND id = $2
		  AND deleted_at IS NULL
		RETURNING id, project_id, name, description, mode, is_active, created_at, updated_at
	`)).
		WithArgs(int64(101), int64(303)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "name", "description", "mode", "is_active", "created_at", "updated_at"}).
			AddRow(int64(303), int64(101), unrestrictedProjectScopeProfileName, "Internal project-level unrestricted resource scope.", service.ProjectProfileModeUnrestricted, true, now, now))
	expectSchedulerFullRebuildOutbox(mock)
	mock.ExpectCommit()

	repo := NewProjectRepository(db)
	project, err := repo.CreateProject(context.Background(), service.ProjectCreateInput{
		Name:        "Ops Space",
		Slug:        "ops-space",
		OwnerUserID: 10,
		ProfileMode: service.ProjectProfileModeUnrestricted,
		Bindings: service.ProjectProfileBindingInput{
			GroupIDs:        []int64{20},
			AccountIDs:      []int64{30},
			ProxyIDs:        []int64{35},
			SubscriptionIDs: []int64{40},
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(101), project.ID)
	require.Equal(t, "Ops Space", project.Name)
	require.Equal(t, "ops-space", project.Slug)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProjectRepositoryActivateUnrestrictedScopeReusesInternalProfile(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id
		FROM project_profiles
		WHERE project_id = $1
		  AND name = $2
		  AND mode = $3
		  AND deleted_at IS NULL
		ORDER BY id ASC
		LIMIT 1
	`)).
		WithArgs(int64(101), unrestrictedProjectScopeProfileName, service.ProjectProfileModeUnrestricted).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(303)))
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE project_profiles
		SET is_active = FALSE,
			updated_at = CURRENT_TIMESTAMP
		WHERE project_id = $1
		  AND is_active = TRUE
		  AND deleted_at IS NULL
	`)).
		WithArgs(int64(101)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`
		UPDATE project_profiles
		SET is_active = TRUE,
			updated_at = CURRENT_TIMESTAMP
		WHERE project_id = $1
		  AND id = $2
		  AND deleted_at IS NULL
		RETURNING id, project_id, name, description, mode, is_active, created_at, updated_at
	`)).
		WithArgs(int64(101), int64(303)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "name", "description", "mode", "is_active", "created_at", "updated_at"}).
			AddRow(int64(303), int64(101), unrestrictedProjectScopeProfileName, "Internal project-level unrestricted resource scope.", service.ProjectProfileModeUnrestricted, true, now, now))
	expectSchedulerFullRebuildOutbox(mock)
	mock.ExpectCommit()

	repo := NewProjectRepository(db)
	profile, err := repo.ActivateProjectUnrestrictedScope(context.Background(), 101)
	require.NoError(t, err)
	require.Equal(t, int64(303), profile.ID)
	require.Equal(t, service.ProjectProfileModeUnrestricted, profile.Mode)
	require.True(t, profile.IsActive)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProjectRepositoryActivateProjectProfileEnqueuesSchedulerRebuild(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id
		FROM project_profiles
		WHERE project_id = $1
		  AND id = $2
		  AND deleted_at IS NULL
	`)).
		WithArgs(int64(101), int64(202)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(202)))
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE project_profiles
		SET is_active = FALSE,
			updated_at = CURRENT_TIMESTAMP
		WHERE project_id = $1
		  AND is_active = TRUE
		  AND deleted_at IS NULL
	`)).
		WithArgs(int64(101)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`
		UPDATE project_profiles
		SET is_active = TRUE,
			updated_at = CURRENT_TIMESTAMP
		WHERE project_id = $1
		  AND id = $2
		  AND deleted_at IS NULL
		RETURNING id, project_id, name, description, mode, is_active, created_at, updated_at
	`)).
		WithArgs(int64(101), int64(202)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "name", "description", "mode", "is_active", "created_at", "updated_at"}).
			AddRow(int64(202), int64(101), "默认配置", nil, service.ProjectProfileModeRestricted, true, now, now))
	expectSchedulerFullRebuildOutbox(mock)
	mock.ExpectCommit()

	repo := NewProjectRepository(db)
	profile, err := repo.ActivateProjectProfile(context.Background(), 101, 202)
	require.NoError(t, err)
	require.Equal(t, int64(202), profile.ID)
	require.True(t, profile.IsActive)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProjectRepositoryListProjectProfilesIncludesUnrestrictedScopeState(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id
		FROM project_profiles
		WHERE project_id = $1
		  AND is_active = TRUE
		  AND deleted_at IS NULL
		LIMIT 1
	`)).
		WithArgs(int64(101)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(303)))
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, project_id, name, description, mode, is_active, created_at, updated_at
		FROM project_profiles
		WHERE project_id = $1
		  AND deleted_at IS NULL
		ORDER BY is_active DESC, id ASC
	`)).
		WithArgs(int64(101)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "name", "description", "mode", "is_active", "created_at", "updated_at"}).
			AddRow(int64(303), int64(101), unrestrictedProjectScopeProfileName, "Internal project-level unrestricted resource scope.", service.ProjectProfileModeUnrestricted, true, now, now).
			AddRow(int64(202), int64(101), "默认配置", "Default active project application profile.", service.ProjectProfileModeRestricted, false, now, now))

	repo := NewProjectRepository(db)
	profiles, err := repo.ListProjectProfiles(context.Background(), 101)
	require.NoError(t, err)
	require.Len(t, profiles, 2)
	require.Equal(t, service.ProjectProfileModeUnrestricted, profiles[0].Mode)
	require.True(t, profiles[0].IsActive)
	require.Equal(t, service.ProjectProfileModeRestricted, profiles[1].Mode)
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
		{service.ProjectResourceTypeGroup, []int64{20}},
		{service.ProjectResourceTypeAccount, []int64{30}},
		{service.ProjectResourceTypeProxy, []int64{35}},
		{service.ProjectResourceTypeSubscription, []int64{40}},
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
			GroupIDs:        []int64{20},
			AccountIDs:      []int64{30},
			ProxyIDs:        []int64{35},
			SubscriptionIDs: []int64{40},
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
		SELECT p.id, p.name, p.slug, p.description, pm.role, pm.is_owner, COALESCE(pm.scopes, '[]'::jsonb)
		FROM project_members pm
		JOIN projects p ON p.id = pm.project_id
		WHERE pm.user_id = $1
		  AND pm.status = 'active'
		  AND p.deleted_at IS NULL
		  AND p.status = 'active'
		ORDER BY pm.is_owner DESC, p.id ASC
	`)).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slug", "description", "role", "is_owner", "scopes"}).
			AddRow(int64(101), "Ops Space", "ops-space", nil, service.ProjectRoleAdmin, true, "[]"))

	repo := NewProjectRepository(db)
	projects, err := repo.ListUserProjects(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, projects, 1)
	require.Equal(t, int64(101), projects[0].ID)
	require.Equal(t, service.ProjectRoleAdmin, projects[0].Role)
	require.True(t, projects[0].IsOwner)
	require.ElementsMatch(t, service.DefaultProjectAdminPermissions(), projects[0].Permissions)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProjectRepositoryListProjectMembersIncludesUserRole(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT
			pm.project_id,
			pm.user_id,
			u.email,
			COALESCE(u.username, ''),
			pm.role,
			u.role,
			pm.is_owner,
			COALESCE(pm.scopes, '[]'::jsonb),
			pm.status,
			u.status,
			pm.created_at,
			pm.updated_at
		FROM project_members pm
		JOIN users u ON u.id = pm.user_id
		WHERE pm.project_id = $1
		  AND u.deleted_at IS NULL
		ORDER BY pm.is_owner DESC, pm.role ASC, u.id ASC
	`)).
		WithArgs(int64(101)).
		WillReturnRows(sqlmock.NewRows([]string{
			"project_id", "user_id", "email", "username", "role", "user_role", "is_owner", "scopes", "status", "user_status", "created_at", "updated_at",
		}).AddRow(int64(101), int64(10), "admin@example.com", "root", service.ProjectRoleAdmin, service.RoleSuperAdmin, true, "[]", service.StatusActive, service.StatusActive, now, now))

	repo := NewProjectRepository(db)
	members, err := repo.ListProjectMembers(context.Background(), 101)
	require.NoError(t, err)
	require.Len(t, members, 1)
	require.Equal(t, service.ProjectRoleAdmin, members[0].Role)
	require.Equal(t, service.RoleSuperAdmin, members[0].UserRole)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProjectRepositorySetActiveMemberOnlyUpdatesMembership(t *testing.T) {
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
			VALUES ($1, $2, $3, $4::jsonb, $5, $6, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			ON CONFLICT (project_id, user_id) DO UPDATE
			SET role = EXCLUDED.role,
				scopes = EXCLUDED.scopes,
				is_owner = EXCLUDED.is_owner,
				status = EXCLUDED.status,
				updated_at = CURRENT_TIMESTAMP
			RETURNING project_id, user_id, role, is_owner, scopes, status, created_at, updated_at
		)
		SELECT
			up.project_id,
			up.user_id,
			u.email,
			COALESCE(u.username, ''),
			up.role,
			u.role,
			up.is_owner,
			up.scopes,
			up.status,
			u.status,
			up.created_at,
			up.updated_at
		FROM upserted up
		JOIN users u ON u.id = up.user_id
	`)).
		WithArgs(int64(101), int64(10), service.ProjectRoleUser, "[]", false, service.StatusActive).
		WillReturnRows(sqlmock.NewRows([]string{
			"project_id", "user_id", "email", "username", "role", "user_role", "is_owner", "scopes", "status", "user_status", "created_at", "updated_at",
		}).AddRow(int64(101), int64(10), "member@example.com", "member", service.ProjectRoleUser, service.RoleSuperAdmin, false, "[]", service.StatusActive, service.StatusActive, now, now))
	mock.ExpectCommit()

	repo := NewProjectRepository(db)
	member, err := repo.SetProjectMember(context.Background(), 101, service.ProjectMemberInput{
		UserID: 10,
		Role:   service.ProjectRoleUser,
	})
	require.NoError(t, err)
	require.Equal(t, int64(10), member.UserID)
	require.Equal(t, service.RoleSuperAdmin, member.UserRole)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProjectRepositoryDisableMemberOnlyUpdatesMembership(t *testing.T) {
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
			VALUES ($1, $2, $3, $4::jsonb, $5, $6, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			ON CONFLICT (project_id, user_id) DO UPDATE
			SET role = EXCLUDED.role,
				scopes = EXCLUDED.scopes,
				is_owner = EXCLUDED.is_owner,
				status = EXCLUDED.status,
				updated_at = CURRENT_TIMESTAMP
			RETURNING project_id, user_id, role, is_owner, scopes, status, created_at, updated_at
		)
		SELECT
			up.project_id,
			up.user_id,
			u.email,
			COALESCE(u.username, ''),
			up.role,
			u.role,
			up.is_owner,
			up.scopes,
			up.status,
			u.status,
			up.created_at,
			up.updated_at
		FROM upserted up
		JOIN users u ON u.id = up.user_id
	`)).
		WithArgs(int64(101), int64(10), service.ProjectRoleUser, "[]", false, service.StatusDisabled).
		WillReturnRows(sqlmock.NewRows([]string{
			"project_id", "user_id", "email", "username", "role", "user_role", "is_owner", "scopes", "status", "user_status", "created_at", "updated_at",
		}).AddRow(int64(101), int64(10), "member@example.com", "member", service.ProjectRoleUser, service.RoleUser, false, "[]", service.StatusDisabled, service.StatusActive, now, now))
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

func TestProjectRepositoryRemoveMemberOnlyDeletesMembership(t *testing.T) {
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
	mock.ExpectCommit()

	repo := NewProjectRepository(db)
	require.NoError(t, repo.RemoveProjectMember(context.Background(), 101, 10))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProjectRepositoryUpdateProjectProfileDoesNotClearBindings(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	name := "Renamed"
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`
		UPDATE project_profiles
		SET name = COALESCE($3, name),
			description = COALESCE($4, description),
			updated_at = CURRENT_TIMESTAMP
		WHERE project_id = $1
		  AND id = $2
		  AND deleted_at IS NULL
		RETURNING id, project_id, name, description, mode, is_active, created_at, updated_at
	`)).
		WithArgs(int64(101), int64(202), name, nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "name", "description", "mode", "is_active", "created_at", "updated_at"}).
			AddRow(int64(202), int64(101), name, nil, service.ProjectProfileModeRestricted, true, now, now))
	mock.ExpectCommit()

	repo := NewProjectRepository(db)
	profile, err := repo.UpdateProjectProfile(context.Background(), 101, 202, service.ProjectProfileInput{Name: &name})
	require.NoError(t, err)
	require.Equal(t, name, profile.Name)
	require.Equal(t, service.ProjectProfileModeRestricted, profile.Mode)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProjectRepositorySetBindingsUpdatesRestrictedProfileBindings(t *testing.T) {
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
		WillReturnRows(sqlmock.NewRows([]string{"mode"}).AddRow(service.ProjectProfileModeRestricted))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM groups WHERE id = ANY\\(\\$1\\) AND deleted_at IS NULL").
		WithArgs(pq.Array([]int64{7})).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM accounts WHERE id = ANY\\(\\$1\\) AND deleted_at IS NULL").
		WithArgs(pq.Array([]int64{8})).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM proxies WHERE id = ANY\\(\\$1\\) AND deleted_at IS NULL").
		WithArgs(pq.Array([]int64{10})).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM user_subscriptions WHERE id = ANY\\(\\$1\\) AND deleted_at IS NULL").
		WithArgs(pq.Array([]int64{9})).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec(regexp.QuoteMeta(`
			DELETE FROM project_profile_bindings
			WHERE project_profile_id = $1
	`)).
		WithArgs(int64(202)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	for _, item := range []struct {
		resourceType string
		ids          []int64
	}{
		{service.ProjectResourceTypeGroup, []int64{7}},
		{service.ProjectResourceTypeAccount, []int64{8}},
		{service.ProjectResourceTypeProxy, []int64{10}},
		{service.ProjectResourceTypeSubscription, []int64{9}},
	} {
		mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO project_profile_bindings (project_profile_id, resource_type, resource_id, created_at)
		SELECT $1, $2, unnest($3::bigint[]), CURRENT_TIMESTAMP
		ON CONFLICT (project_profile_id, resource_type, resource_id) DO NOTHING
	`)).
			WithArgs(int64(202), item.resourceType, pq.Array(item.ids)).
			WillReturnResult(sqlmock.NewResult(0, int64(len(item.ids))))
	}
	mock.ExpectExec(regexp.QuoteMeta(`
			UPDATE project_profiles
			SET updated_at = CURRENT_TIMESTAMP
			WHERE id = $1
		`)).
		WithArgs(int64(202)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectSchedulerFullRebuildOutbox(mock)
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT mode
		FROM project_profiles
		WHERE project_id = $1
		  AND id = $2
		  AND deleted_at IS NULL
	`)).
		WithArgs(int64(101), int64(202)).
		WillReturnRows(sqlmock.NewRows([]string{"mode"}).AddRow(service.ProjectProfileModeRestricted))
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT resource_type, resource_id
		FROM project_profile_bindings
		WHERE project_profile_id = $1
		ORDER BY resource_type ASC, resource_id ASC
	`)).
		WithArgs(int64(202)).
		WillReturnRows(sqlmock.NewRows([]string{"resource_type", "resource_id"}).
			AddRow(service.ProjectResourceTypeAccount, int64(8)).
			AddRow(service.ProjectResourceTypeGroup, int64(7)).
			AddRow(service.ProjectResourceTypeProxy, int64(10)).
			AddRow(service.ProjectResourceTypeSubscription, int64(9)))
	expectProjectBindingDetails(t, mock, int64(101), []int64{7}, []int64{8}, []int64{10}, []int64{9})

	repo := NewProjectRepository(db)
	bindings, err := repo.SetProjectProfileBindings(context.Background(), 101, 202, service.ProjectProfileBindingInput{
		GroupIDs:        []int64{7},
		AccountIDs:      []int64{8},
		ProxyIDs:        []int64{10},
		SubscriptionIDs: []int64{9},
	})
	require.NoError(t, err)
	require.Equal(t, int64(202), bindings.ProfileID)
	require.Equal(t, []int64{7}, bindings.GroupIDs)
	require.Equal(t, []int64{8}, bindings.AccountIDs)
	require.Equal(t, []int64{10}, bindings.ProxyIDs)
	require.Equal(t, []int64{9}, bindings.SubscriptionIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProjectRepositoryGetProfileBindingsIncludesResourceDetails(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT mode
		FROM project_profiles
		WHERE project_id = $1
		  AND id = $2
		  AND deleted_at IS NULL
	`)).
		WithArgs(int64(101), int64(202)).
		WillReturnRows(sqlmock.NewRows([]string{"mode"}).AddRow(service.ProjectProfileModeRestricted))
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT resource_type, resource_id
		FROM project_profile_bindings
		WHERE project_profile_id = $1
		ORDER BY resource_type ASC, resource_id ASC
	`)).
		WithArgs(int64(202)).
		WillReturnRows(sqlmock.NewRows([]string{"resource_type", "resource_id"}).
			AddRow(service.ProjectResourceTypeAccount, int64(8)).
			AddRow(service.ProjectResourceTypeGroup, int64(7)).
			AddRow(service.ProjectResourceTypeProxy, int64(10)).
			AddRow(service.ProjectResourceTypeSubscription, int64(9)))
	expectProjectBindingDetails(t, mock, int64(101), []int64{7}, []int64{8}, []int64{10}, []int64{9})

	repo := NewProjectRepository(db)
	bindings, err := repo.GetProjectProfileBindings(context.Background(), 101, 202)
	require.NoError(t, err)
	require.Equal(t, []int64{7}, bindings.GroupIDs)
	require.Equal(t, []int64{8}, bindings.AccountIDs)
	require.Equal(t, []int64{10}, bindings.ProxyIDs)
	require.Equal(t, []int64{9}, bindings.SubscriptionIDs)
	require.Len(t, bindings.Groups, 1)
	require.Equal(t, "共享分组", bindings.Groups[0].Name)
	require.Len(t, bindings.Accounts, 1)
	require.Equal(t, "主账号", bindings.Accounts[0].Name)
	require.Len(t, bindings.Proxies, 1)
	require.Equal(t, "主代理", bindings.Proxies[0].Name)
	require.Len(t, bindings.Subscriptions, 1)
	require.Equal(t, "user@example.test", bindings.Subscriptions[0].UserEmail)
	require.NoError(t, mock.ExpectationsWereMet())
}

func expectProjectBindingDetails(t *testing.T, mock sqlmock.Sqlmock, projectID int64, groupIDs, accountIDs, proxyIDs, subscriptionIDs []int64) {
	t.Helper()
	if len(groupIDs) > 0 {
		mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, project_id, name, COALESCE(description, ''), platform, status
		FROM groups
		WHERE id = ANY($1)
		  AND deleted_at IS NULL
		ORDER BY id ASC
	`)).
			WithArgs(pq.Array(groupIDs)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "name", "description", "platform", "status"}).
				AddRow(groupIDs[0], projectID, "共享分组", "", "openai", service.StatusActive))
	}
	if len(accountIDs) > 0 {
		mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT
			id,
			project_id,
			name,
			COALESCE(notes, ''),
			platform,
			type,
			status,
			COALESCE(credentials->>'email', credentials->>'account_email', credentials->>'user_email', extra->>'email', '')
		FROM accounts
		WHERE id = ANY($1)
		  AND deleted_at IS NULL
		ORDER BY id ASC
	`)).
			WithArgs(pq.Array(accountIDs)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "name", "notes", "platform", "type", "status", "email"}).
				AddRow(accountIDs[0], projectID, "主账号", "", "openai", "api_key", service.StatusActive, "owner@example.test"))
	}
	if len(proxyIDs) > 0 {
		mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, project_id, name, protocol, host, port, status
		FROM proxies
		WHERE id = ANY($1)
		  AND deleted_at IS NULL
		ORDER BY id ASC
	`)).
			WithArgs(pq.Array(proxyIDs)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "name", "protocol", "host", "port", "status"}).
				AddRow(proxyIDs[0], projectID, "主代理", "http", "proxy.example.test", 8080, service.StatusActive))
	}
	if len(subscriptionIDs) > 0 {
		mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT us.id, us.user_id, us.group_id, u.email, g.name, us.status, COALESCE(us.notes, '')
		FROM user_subscriptions us
		JOIN users u ON u.id = us.user_id
		JOIN groups g ON g.id = us.group_id
		WHERE us.id = ANY($1)
		  AND us.deleted_at IS NULL
		  AND u.deleted_at IS NULL
		  AND g.deleted_at IS NULL
		ORDER BY us.id ASC
	`)).
			WithArgs(pq.Array(subscriptionIDs)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "group_id", "user_email", "group_name", "status", "notes"}).
				AddRow(subscriptionIDs[0], int64(42), groupIDs[0], "user@example.test", "共享分组", service.StatusActive, "包月"))
	}
}

func TestProjectRepositoryValidateProjectProfileBindingScopeRejectsInvisibleResource(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM groups").
		WithArgs(pq.Array([]int64{99})).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	repo := NewProjectRepository(db)
	err = repo.ValidateProjectProfileBindingScope(context.Background(), 101, service.ProjectProfileBindingInput{
		GroupIDs: []int64{99},
	})
	require.ErrorIs(t, err, service.ErrProjectAccessForbidden)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProjectRepositoryValidateProjectProfileBindingResourcesRejectsMissingResource(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM groups WHERE id = ANY\\(\\$1\\) AND deleted_at IS NULL").
		WithArgs(pq.Array([]int64{99})).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	repo := NewProjectRepository(db)
	err = repo.ValidateProjectProfileBindingResources(context.Background(), service.ProjectProfileBindingInput{
		GroupIDs: []int64{99},
	})
	require.ErrorIs(t, err, service.ErrProjectInvalidInput)
	require.NoError(t, mock.ExpectationsWereMet())
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
