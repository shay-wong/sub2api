package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

const unrestrictedProjectScopeProfileName = "__unrestricted__"

type projectRepository struct {
	sql sqlExecutor
}

func NewProjectRepository(sqlDB *sql.DB) service.ProjectRepository {
	return &projectRepository{sql: sqlDB}
}

func (r *projectRepository) GetDefaultProjectID(ctx context.Context) (int64, error) {
	return ensureDefaultProject(ctx, r.sql)
}

func (r *projectRepository) ProjectExists(ctx context.Context, projectID int64) (bool, error) {
	if r == nil || r.sql == nil {
		return false, fmt.Errorf("nil project repository")
	}
	if projectID <= 0 {
		return false, nil
	}
	var id int64
	err := scanSingleRow(ctx, r.sql, `
		SELECT id
		FROM projects
		WHERE id = $1
		  AND deleted_at IS NULL
		  AND status = 'active'
		LIMIT 1
	`, []any{projectID}, &id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return id > 0, nil
}

func (r *projectRepository) GetProjectRole(ctx context.Context, projectID int64, userID int64) (string, bool, error) {
	if r == nil || r.sql == nil {
		return "", false, fmt.Errorf("nil project repository")
	}
	if projectID <= 0 || userID <= 0 {
		return "", false, nil
	}
	var role string
	err := scanSingleRow(ctx, r.sql, `
		SELECT pm.role
		FROM project_members pm
		JOIN projects p ON p.id = pm.project_id
		WHERE pm.project_id = $1
		  AND pm.user_id = $2
		  AND pm.status = 'active'
		  AND p.deleted_at IS NULL
		  AND p.status = 'active'
		LIMIT 1
	`, []any{projectID, userID}, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return role, true, nil
}

func (r *projectRepository) ListActiveProjects(ctx context.Context) ([]service.ProjectSummary, error) {
	if r == nil || r.sql == nil {
		return nil, fmt.Errorf("nil project repository")
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, name, slug, description
		FROM projects
		WHERE deleted_at IS NULL
		  AND status = 'active'
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.ProjectSummary, 0)
	for rows.Next() {
		var item service.ProjectSummary
		var description sql.NullString
		if scanErr := rows.Scan(&item.ID, &item.Name, &item.Slug, &description); scanErr != nil {
			return nil, scanErr
		}
		if description.Valid {
			item.Description = &description.String
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *projectRepository) ListUserProjects(ctx context.Context, userID int64) ([]service.ProjectSummary, error) {
	if r == nil || r.sql == nil {
		return nil, fmt.Errorf("nil project repository")
	}
	if userID <= 0 {
		return []service.ProjectSummary{}, nil
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT p.id, p.name, p.slug, p.description, pm.role, pm.is_owner, COALESCE(pm.scopes, '[]'::jsonb)
		FROM project_members pm
		JOIN projects p ON p.id = pm.project_id
		WHERE pm.user_id = $1
		  AND pm.status = 'active'
		  AND p.deleted_at IS NULL
		  AND p.status = 'active'
		ORDER BY pm.is_owner DESC, p.id ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.ProjectSummary, 0)
	for rows.Next() {
		var item service.ProjectSummary
		var description sql.NullString
		var scopes any
		if scanErr := rows.Scan(&item.ID, &item.Name, &item.Slug, &description, &item.Role, &item.IsOwner, &scopes); scanErr != nil {
			return nil, scanErr
		}
		if description.Valid {
			item.Description = &description.String
		}
		item.Permissions = service.ProjectAdminPermissionsForDisplay(item.Role, decodeProjectMemberScopes(scopes))
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *projectRepository) CreateProject(ctx context.Context, input service.ProjectCreateInput) (*service.ProjectSummary, error) {
	if r == nil || r.sql == nil {
		return nil, fmt.Errorf("nil project repository")
	}
	tx, err := beginSQLTx(ctx, r.sql)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	project, err := insertProject(ctx, tx, input)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO project_members (project_id, user_id, role, scopes, is_owner, created_at, updated_at)
		VALUES ($1, $2, $3, '[]', TRUE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (project_id, user_id) DO UPDATE
		SET role = EXCLUDED.role,
			is_owner = TRUE,
			updated_at = CURRENT_TIMESTAMP
	`, project.ID, input.OwnerUserID, service.ProjectRoleAdmin); err != nil {
		return nil, err
	}
	var profileID int64
	if err := scanSingleRow(ctx, tx, `
		INSERT INTO project_profiles (project_id, name, description, mode, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, TRUE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT DO NOTHING
		RETURNING id
		`, []any{project.ID, "默认配置", "Default active project application profile.", service.ProjectProfileModeRestricted}, &profileID); err != nil {
		return nil, err
	}
	if err := insertProjectProfileBindings(ctx, tx, profileID, service.ProjectResourceTypeGroup, input.Bindings.GroupIDs); err != nil {
		return nil, err
	}
	if err := insertProjectProfileBindings(ctx, tx, profileID, service.ProjectResourceTypeAccount, input.Bindings.AccountIDs); err != nil {
		return nil, err
	}
	if err := insertProjectProfileBindings(ctx, tx, profileID, service.ProjectResourceTypeProxy, input.Bindings.ProxyIDs); err != nil {
		return nil, err
	}
	if err := insertProjectProfileBindings(ctx, tx, profileID, service.ProjectResourceTypeSubscription, input.Bindings.SubscriptionIDs); err != nil {
		return nil, err
	}
	if input.ProfileMode == service.ProjectProfileModeUnrestricted {
		if _, err := activateProjectUnrestrictedScopeOnTx(ctx, tx, project.ID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return project, nil
}

func insertProject(ctx context.Context, tx *sql.Tx, input service.ProjectCreateInput) (*service.ProjectSummary, error) {
	var project service.ProjectSummary
	var description sql.NullString
	err := scanSingleRow(ctx, tx, `
		INSERT INTO projects (name, slug, description, status, profiles, created_at, updated_at)
		VALUES ($1, $2, $3, 'active', '{}'::jsonb, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING id, name, slug, description
	`, []any{input.Name, input.Slug, nullableString(input.Description)}, &project.ID, &project.Name, &project.Slug, &description)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, service.ErrProjectSlugConflict
		}
		return nil, err
	}
	if description.Valid {
		project.Description = &description.String
	}
	return &project, nil
}

func (r *projectRepository) UpdateProject(ctx context.Context, projectID int64, input service.ProjectUpdateInput) (*service.ProjectSummary, error) {
	if r == nil || r.sql == nil {
		return nil, fmt.Errorf("nil project repository")
	}
	var project service.ProjectSummary
	var description sql.NullString
	err := scanSingleRow(ctx, r.sql, `
		UPDATE projects
		SET name = COALESCE($2, name),
			description = COALESCE($3, description),
			status = COALESCE($4, status),
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		  AND deleted_at IS NULL
		RETURNING id, name, slug, description
	`, []any{
		projectID,
		nullableString(input.Name),
		nullableString(input.Description),
		nullableString(input.Status),
	}, &project.ID, &project.Name, &project.Slug, &description)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrProjectNotFound
	}
	if err != nil {
		return nil, err
	}
	if description.Valid {
		project.Description = &description.String
	}
	return &project, nil
}

func (r *projectRepository) ListProjectMembers(ctx context.Context, projectID int64) ([]service.ProjectMember, error) {
	if r == nil || r.sql == nil {
		return nil, fmt.Errorf("nil project repository")
	}
	rows, err := r.sql.QueryContext(ctx, `
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
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.ProjectMember, 0)
	for rows.Next() {
		var item service.ProjectMember
		var scopes any
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&item.ProjectID, &item.UserID, &item.Email, &item.Username, &item.Role, &item.UserRole, &item.IsOwner, &scopes, &item.Status, &item.UserStatus, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		item.Permissions = service.ProjectAdminPermissionsForDisplay(item.Role, decodeProjectMemberScopes(scopes))
		item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *projectRepository) SetProjectMember(ctx context.Context, projectID int64, input service.ProjectMemberInput) (*service.ProjectMember, error) {
	if r == nil || r.sql == nil {
		return nil, fmt.Errorf("nil project repository")
	}
	tx, err := beginSQLTx(ctx, r.sql)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var exists int64
	if err := scanSingleRow(ctx, tx, `
		SELECT id FROM users WHERE id = $1 AND deleted_at IS NULL LIMIT 1
	`, []any{input.UserID}, &exists); errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrPermissionUserNotFound
	} else if err != nil {
		return nil, err
	}

	status := service.StatusActive
	if input.Status != nil {
		status = *input.Status
	}
	permissions := input.Permissions
	if permissions == nil {
		permissions = []string{}
	}
	scopesJSON, err := json.Marshal(permissions)
	if err != nil {
		return nil, err
	}
	if input.IsOwner {
		if _, err := tx.ExecContext(ctx, `
			UPDATE project_members
			SET is_owner = FALSE,
				updated_at = CURRENT_TIMESTAMP
			WHERE project_id = $1
			  AND user_id <> $2
			  AND is_owner = TRUE
		`, projectID, input.UserID); err != nil {
			return nil, err
		}
	}

	var item service.ProjectMember
	var scopes any
	var createdAt, updatedAt time.Time
	err = scanSingleRow(ctx, tx, `
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
	`, []any{projectID, input.UserID, input.Role, string(scopesJSON), input.IsOwner, status}, &item.ProjectID, &item.UserID, &item.Email, &item.Username, &item.Role, &item.UserRole, &item.IsOwner, &scopes, &item.Status, &item.UserStatus, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	item.Permissions = service.ProjectAdminPermissionsForDisplay(item.Role, decodeProjectMemberScopes(scopes))
	item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	return &item, nil
}

func (r *projectRepository) RemoveProjectMember(ctx context.Context, projectID int64, userID int64) error {
	if r == nil || r.sql == nil {
		return fmt.Errorf("nil project repository")
	}
	var isOwner bool
	if err := scanSingleRow(ctx, r.sql, `
		SELECT is_owner
		FROM project_members
		WHERE project_id = $1
		  AND user_id = $2
	`, []any{projectID, userID}, &isOwner); errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	if isOwner {
		return service.ErrProjectOwnerTransferRequired
	}
	tx, err := beginSQLTx(ctx, r.sql)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
		DELETE FROM project_members
		WHERE project_id = $1
		  AND user_id = $2
	`, projectID, userID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func decodeProjectMemberScopes(raw any) []string {
	switch v := raw.(type) {
	case nil:
		return []string{}
	case []string:
		return append([]string(nil), v...)
	case []byte:
		return decodeProjectMemberScopeBytes(v)
	case string:
		return decodeProjectMemberScopeBytes([]byte(v))
	case fmt.Stringer:
		return decodeProjectMemberScopeBytes([]byte(v.String()))
	default:
		return []string{}
	}
}

func decodeProjectMemberScopeBytes(raw []byte) []string {
	if len(raw) == 0 {
		return []string{}
	}
	var scopes []string
	if err := json.Unmarshal(raw, &scopes); err != nil {
		return []string{}
	}
	return scopes
}

func (r *projectRepository) ListProjectProfiles(ctx context.Context, projectID int64) ([]service.ProjectProfile, error) {
	if r == nil || r.sql == nil {
		return nil, fmt.Errorf("nil project repository")
	}
	if err := ensureProjectActiveProfile(ctx, r.sql, projectID); err != nil {
		return nil, err
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, project_id, name, description, mode, is_active, created_at, updated_at
		FROM project_profiles
		WHERE project_id = $1
		  AND deleted_at IS NULL
		ORDER BY is_active DESC, id ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.ProjectProfile, 0)
	for rows.Next() {
		var item service.ProjectProfile
		var description sql.NullString
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&item.ID, &item.ProjectID, &item.Name, &description, &item.Mode, &item.IsActive, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if description.Valid {
			item.Description = &description.String
		}
		item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *projectRepository) CreateProjectProfile(ctx context.Context, projectID int64, input service.ProjectProfileInput) (*service.ProjectProfile, error) {
	if r == nil || r.sql == nil {
		return nil, fmt.Errorf("nil project repository")
	}
	name := derefString(input.Name)
	var item service.ProjectProfile
	var description sql.NullString
	var createdAt, updatedAt time.Time
	err := scanSingleRow(ctx, r.sql, `
		INSERT INTO project_profiles (project_id, name, description, mode, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, FALSE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING id, project_id, name, description, mode, is_active, created_at, updated_at
	`, []any{projectID, name, nullableString(input.Description), service.ProjectProfileModeRestricted}, &item.ID, &item.ProjectID, &item.Name, &description, &item.Mode, &item.IsActive, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if description.Valid {
		item.Description = &description.String
	}
	item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	return &item, nil
}

func (r *projectRepository) ActivateProjectUnrestrictedScope(ctx context.Context, projectID int64) (*service.ProjectProfile, error) {
	if r == nil || r.sql == nil {
		return nil, fmt.Errorf("nil project repository")
	}
	tx, err := beginSQLTx(ctx, r.sql)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	item, err := activateProjectUnrestrictedScopeOnTx(ctx, tx, projectID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return item, nil
}

func (r *projectRepository) UpdateProjectProfile(ctx context.Context, projectID int64, profileID int64, input service.ProjectProfileInput) (*service.ProjectProfile, error) {
	if r == nil || r.sql == nil {
		return nil, fmt.Errorf("nil project repository")
	}
	tx, err := beginSQLTx(ctx, r.sql)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var item service.ProjectProfile
	var description sql.NullString
	var createdAt, updatedAt time.Time
	err = scanSingleRow(ctx, tx, `
		UPDATE project_profiles
		SET name = COALESCE($3, name),
			description = COALESCE($4, description),
			updated_at = CURRENT_TIMESTAMP
		WHERE project_id = $1
		  AND id = $2
		  AND deleted_at IS NULL
		RETURNING id, project_id, name, description, mode, is_active, created_at, updated_at
	`, []any{projectID, profileID, nullableString(input.Name), nullableString(input.Description)}, &item.ID, &item.ProjectID, &item.Name, &description, &item.Mode, &item.IsActive, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrProjectProfileNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if description.Valid {
		item.Description = &description.String
	}
	item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	return &item, nil
}

func (r *projectRepository) DeleteProjectProfile(ctx context.Context, projectID int64, profileID int64) error {
	if r == nil || r.sql == nil {
		return fmt.Errorf("nil project repository")
	}
	var active bool
	if err := scanSingleRow(ctx, r.sql, `
		SELECT is_active
		FROM project_profiles
		WHERE project_id = $1
		  AND id = $2
		  AND deleted_at IS NULL
	`, []any{projectID, profileID}, &active); errors.Is(err, sql.ErrNoRows) {
		return service.ErrProjectProfileNotFound
	} else if err != nil {
		return err
	}
	if active {
		return service.ErrProjectInvalidInput
	}
	res, err := r.sql.ExecContext(ctx, `
		UPDATE project_profiles
		SET deleted_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP,
			is_active = FALSE
		WHERE project_id = $1
		  AND id = $2
		  AND deleted_at IS NULL
	`, projectID, profileID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return service.ErrProjectProfileNotFound
	}
	return nil
}

func (r *projectRepository) ActivateProjectProfile(ctx context.Context, projectID int64, profileID int64) (*service.ProjectProfile, error) {
	if r == nil || r.sql == nil {
		return nil, fmt.Errorf("nil project repository")
	}
	tx, err := beginSQLTx(ctx, r.sql)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var exists int64
	if err := scanSingleRow(ctx, tx, `
		SELECT id
		FROM project_profiles
		WHERE project_id = $1
		  AND id = $2
		  AND deleted_at IS NULL
	`, []any{projectID, profileID}, &exists); errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrProjectProfileNotFound
	} else if err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE project_profiles
		SET is_active = FALSE,
			updated_at = CURRENT_TIMESTAMP
		WHERE project_id = $1
		  AND is_active = TRUE
		  AND deleted_at IS NULL
	`, projectID); err != nil {
		return nil, err
	}

	var item service.ProjectProfile
	var description sql.NullString
	var createdAt, updatedAt time.Time
	err = scanSingleRow(ctx, tx, `
		UPDATE project_profiles
		SET is_active = TRUE,
			updated_at = CURRENT_TIMESTAMP
		WHERE project_id = $1
		  AND id = $2
		  AND deleted_at IS NULL
		RETURNING id, project_id, name, description, mode, is_active, created_at, updated_at
	`, []any{projectID, profileID}, &item.ID, &item.ProjectID, &item.Name, &description, &item.Mode, &item.IsActive, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if err := enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventFullRebuild, nil, nil, nil); err != nil {
		logger.LegacyPrintf("repository.project", "[SchedulerOutbox] enqueue profile activation rebuild failed: project=%d profile=%d err=%v", projectID, profileID, err)
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if description.Valid {
		item.Description = &description.String
	}
	item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	return &item, nil
}

func activateProjectUnrestrictedScopeOnTx(ctx context.Context, tx *sql.Tx, projectID int64) (*service.ProjectProfile, error) {
	var profileID int64
	err := scanSingleRow(ctx, tx, `
		SELECT id
		FROM project_profiles
		WHERE project_id = $1
		  AND name = $2
		  AND mode = $3
		  AND deleted_at IS NULL
		ORDER BY id ASC
		LIMIT 1
	`, []any{projectID, unrestrictedProjectScopeProfileName, service.ProjectProfileModeUnrestricted}, &profileID)
	if errors.Is(err, sql.ErrNoRows) {
		err = scanSingleRow(ctx, tx, `
			INSERT INTO project_profiles (project_id, name, description, mode, is_active, created_at, updated_at)
			VALUES ($1, $2, $3, $4, FALSE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			RETURNING id
		`, []any{projectID, unrestrictedProjectScopeProfileName, "Internal project-level unrestricted resource scope.", service.ProjectProfileModeUnrestricted}, &profileID)
	}
	if err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE project_profiles
		SET is_active = FALSE,
			updated_at = CURRENT_TIMESTAMP
		WHERE project_id = $1
		  AND is_active = TRUE
		  AND deleted_at IS NULL
	`, projectID); err != nil {
		return nil, err
	}

	var item service.ProjectProfile
	var description sql.NullString
	var createdAt, updatedAt time.Time
	err = scanSingleRow(ctx, tx, `
		UPDATE project_profiles
		SET is_active = TRUE,
			updated_at = CURRENT_TIMESTAMP
		WHERE project_id = $1
		  AND id = $2
		  AND deleted_at IS NULL
		RETURNING id, project_id, name, description, mode, is_active, created_at, updated_at
	`, []any{projectID, profileID}, &item.ID, &item.ProjectID, &item.Name, &description, &item.Mode, &item.IsActive, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if err := enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventFullRebuild, nil, nil, nil); err != nil {
		logger.LegacyPrintf("repository.project", "[SchedulerOutbox] enqueue unrestricted profile activation rebuild failed: project=%d profile=%d err=%v", projectID, profileID, err)
		return nil, err
	}
	if description.Valid {
		item.Description = &description.String
	}
	item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	return &item, nil
}

func (r *projectRepository) GetProjectProfileBindings(ctx context.Context, projectID int64, profileID int64) (*service.ProjectProfileBindings, error) {
	if r == nil || r.sql == nil {
		return nil, fmt.Errorf("nil project repository")
	}
	if err := ensureProjectProfileExists(ctx, r.sql, projectID, profileID); err != nil {
		return nil, err
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT resource_type, resource_id
		FROM project_profile_bindings
		WHERE project_profile_id = $1
		ORDER BY resource_type ASC, resource_id ASC
	`, profileID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := &service.ProjectProfileBindings{ProfileID: profileID}
	for rows.Next() {
		var typ string
		var id int64
		if err := rows.Scan(&typ, &id); err != nil {
			return nil, err
		}
		switch typ {
		case service.ProjectResourceTypeGroup:
			out.GroupIDs = append(out.GroupIDs, id)
		case service.ProjectResourceTypeAccount:
			out.AccountIDs = append(out.AccountIDs, id)
		case service.ProjectResourceTypeProxy:
			out.ProxyIDs = append(out.ProxyIDs, id)
		case service.ProjectResourceTypeSubscription:
			out.SubscriptionIDs = append(out.SubscriptionIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := r.fillProjectProfileBindingDetails(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *projectRepository) fillProjectProfileBindingDetails(ctx context.Context, bindings *service.ProjectProfileBindings) error {
	if r == nil || r.sql == nil || bindings == nil {
		return nil
	}
	if len(bindings.GroupIDs) > 0 {
		groups, err := r.projectBindingGroups(ctx, bindings.GroupIDs)
		if err != nil {
			return err
		}
		bindings.Groups = groups
	}
	if len(bindings.AccountIDs) > 0 {
		accounts, err := r.projectBindingAccounts(ctx, bindings.AccountIDs)
		if err != nil {
			return err
		}
		bindings.Accounts = accounts
	}
	if len(bindings.ProxyIDs) > 0 {
		proxies, err := r.projectBindingProxies(ctx, bindings.ProxyIDs)
		if err != nil {
			return err
		}
		bindings.Proxies = proxies
	}
	if len(bindings.SubscriptionIDs) > 0 {
		subscriptions, err := r.projectBindingSubscriptions(ctx, bindings.SubscriptionIDs)
		if err != nil {
			return err
		}
		bindings.Subscriptions = subscriptions
	}
	return nil
}

func (r *projectRepository) projectBindingGroups(ctx context.Context, ids []int64) ([]service.ProjectResourceGroupCandidate, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, project_id, name, COALESCE(description, ''), platform, status
		FROM groups
		WHERE id = ANY($1)
		  AND deleted_at IS NULL
		ORDER BY id ASC
	`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.ProjectResourceGroupCandidate, 0, len(ids))
	for rows.Next() {
		var item service.ProjectResourceGroupCandidate
		if err := rows.Scan(&item.ID, &item.ProjectID, &item.Name, &item.Description, &item.Platform, &item.Status); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *projectRepository) projectBindingAccounts(ctx context.Context, ids []int64) ([]service.ProjectResourceAccountCandidate, error) {
	rows, err := r.sql.QueryContext(ctx, `
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
	`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.ProjectResourceAccountCandidate, 0, len(ids))
	for rows.Next() {
		var item service.ProjectResourceAccountCandidate
		if err := rows.Scan(&item.ID, &item.ProjectID, &item.Name, &item.Notes, &item.Platform, &item.Type, &item.Status, &item.Email); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *projectRepository) projectBindingProxies(ctx context.Context, ids []int64) ([]service.ProjectResourceProxyCandidate, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, project_id, name, protocol, host, port, status
		FROM proxies
		WHERE id = ANY($1)
		  AND deleted_at IS NULL
		ORDER BY id ASC
	`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.ProjectResourceProxyCandidate, 0, len(ids))
	for rows.Next() {
		var item service.ProjectResourceProxyCandidate
		if err := rows.Scan(&item.ID, &item.ProjectID, &item.Name, &item.Protocol, &item.Host, &item.Port, &item.Status); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *projectRepository) projectBindingSubscriptions(ctx context.Context, ids []int64) ([]service.ProjectResourceSubscriptionCandidate, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT us.id, us.user_id, us.group_id, u.email, g.name, us.status, COALESCE(us.notes, '')
		FROM user_subscriptions us
		JOIN users u ON u.id = us.user_id
		JOIN groups g ON g.id = us.group_id
		WHERE us.id = ANY($1)
		  AND us.deleted_at IS NULL
		  AND u.deleted_at IS NULL
		  AND g.deleted_at IS NULL
		ORDER BY us.id ASC
	`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.ProjectResourceSubscriptionCandidate, 0, len(ids))
	for rows.Next() {
		var item service.ProjectResourceSubscriptionCandidate
		if err := rows.Scan(&item.ID, &item.UserID, &item.GroupID, &item.UserEmail, &item.GroupName, &item.Status, &item.Notes); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *projectRepository) SetProjectProfileBindings(ctx context.Context, projectID int64, profileID int64, input service.ProjectProfileBindingInput) (*service.ProjectProfileBindings, error) {
	if r == nil || r.sql == nil {
		return nil, fmt.Errorf("nil project repository")
	}
	tx, err := beginSQLTx(ctx, r.sql)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := getProjectProfileMode(ctx, tx, projectID, profileID); err != nil {
		return nil, err
	}
	if err := validateProjectBindingIDs(ctx, tx, input); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM project_profile_bindings
		WHERE project_profile_id = $1
	`, profileID); err != nil {
		return nil, err
	}
	if err := insertProjectProfileBindings(ctx, tx, profileID, service.ProjectResourceTypeGroup, input.GroupIDs); err != nil {
		return nil, err
	}
	if err := insertProjectProfileBindings(ctx, tx, profileID, service.ProjectResourceTypeAccount, input.AccountIDs); err != nil {
		return nil, err
	}
	if err := insertProjectProfileBindings(ctx, tx, profileID, service.ProjectResourceTypeProxy, input.ProxyIDs); err != nil {
		return nil, err
	}
	if err := insertProjectProfileBindings(ctx, tx, profileID, service.ProjectResourceTypeSubscription, input.SubscriptionIDs); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE project_profiles
		SET updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, profileID); err != nil {
		return nil, err
	}
	if err := enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventFullRebuild, nil, nil, nil); err != nil {
		logger.LegacyPrintf("repository.project", "[SchedulerOutbox] enqueue profile binding rebuild failed: project=%d profile=%d err=%v", projectID, profileID, err)
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetProjectProfileBindings(ctx, projectID, profileID)
}

func (r *projectRepository) ValidateProjectProfileBindingScope(ctx context.Context, projectID int64, input service.ProjectProfileBindingInput) error {
	if r == nil || r.sql == nil {
		return fmt.Errorf("nil project repository")
	}
	checks := []struct {
		table     string
		ids       []int64
		extra     string
		resources projectSQLScopeResources
	}{
		{
			table:     "groups",
			ids:       input.GroupIDs,
			extra:     "deleted_at IS NULL",
			resources: projectSQLScopeResources{GroupID: "groups.id"},
		},
		{
			table:     "accounts",
			ids:       input.AccountIDs,
			extra:     "deleted_at IS NULL",
			resources: projectSQLScopeResources{AccountID: "accounts.id"},
		},
		{
			table:     "proxies",
			ids:       input.ProxyIDs,
			extra:     "deleted_at IS NULL",
			resources: projectSQLScopeResources{ProxyID: "proxies.id"},
		},
		{
			table:     "user_subscriptions",
			ids:       input.SubscriptionIDs,
			extra:     "deleted_at IS NULL",
			resources: projectSQLScopeResources{SubscriptionID: "user_subscriptions.id", UserID: "user_subscriptions.user_id", GroupID: "user_subscriptions.group_id"},
		},
	}
	for _, check := range checks {
		if len(check.ids) == 0 {
			continue
		}
		scopeSQL := projectProfileScopeSQL(projectID, check.resources)
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE id = ANY($1) AND %s AND %s", check.table, check.extra, scopeSQL)
		var count int
		if err := scanSingleRow(ctx, r.sql, query, []any{pq.Array(check.ids)}, &count); err != nil {
			return err
		}
		if count != len(check.ids) {
			return service.ErrProjectAccessForbidden
		}
	}
	return nil
}

func (r *projectRepository) ValidateProjectProfileBindingResources(ctx context.Context, input service.ProjectProfileBindingInput) error {
	if r == nil || r.sql == nil {
		return fmt.Errorf("nil project repository")
	}
	return validateProjectBindingIDs(ctx, r.sql, input)
}

func (r *projectRepository) SearchProjectBindableResources(ctx context.Context, projectID int64, query string, limit int) (*service.ProjectResourceSearchResult, error) {
	if r == nil || r.sql == nil {
		return nil, fmt.Errorf("nil project repository")
	}
	if limit <= 0 {
		limit = 20
	}
	like := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	out := &service.ProjectResourceSearchResult{
		Users:         []service.ProjectResourceUserCandidate{},
		Groups:        []service.ProjectResourceGroupCandidate{},
		Accounts:      []service.ProjectResourceAccountCandidate{},
		Proxies:       []service.ProjectResourceProxyCandidate{},
		Subscriptions: []service.ProjectResourceSubscriptionCandidate{},
		APIKeys:       []service.ProjectResourceAPIKeyCandidate{},
	}

	userRows, err := r.sql.QueryContext(ctx, `
		SELECT id, email, COALESCE(username, ''), COALESCE(notes, ''), status
		FROM users
		WHERE deleted_at IS NULL
		  AND (
			$1 = '%%'
			OR LOWER(email) LIKE $1
			OR LOWER(COALESCE(username, '')) LIKE $1
			OR LOWER(COALESCE(notes, '')) LIKE $1
		  )
		  `+projectSearchUserScopeCondition(projectID)+`
		ORDER BY id ASC
		LIMIT $2
	`, like, limit)
	if err != nil {
		return nil, err
	}
	for userRows.Next() {
		var item service.ProjectResourceUserCandidate
		if err := userRows.Scan(&item.ID, &item.Email, &item.Username, &item.Notes, &item.Status); err != nil {
			_ = userRows.Close()
			return nil, err
		}
		out.Users = append(out.Users, item)
	}
	if err := userRows.Close(); err != nil {
		return nil, err
	}
	if err := userRows.Err(); err != nil {
		return nil, err
	}

	groupRows, err := r.sql.QueryContext(ctx, `
		SELECT id, project_id, name, COALESCE(description, ''), platform, status
		FROM groups
		WHERE deleted_at IS NULL
		  AND (
			$1 = '%%'
			OR LOWER(name) LIKE $1
			OR LOWER(COALESCE(description, '')) LIKE $1
			OR LOWER(platform) LIKE $1
		  )
		  `+projectSearchScopeCondition(projectID, projectSQLScopeResources{GroupID: "groups.id"})+`
		ORDER BY id ASC
		LIMIT $2
	`, like, limit)
	if err != nil {
		return nil, err
	}
	for groupRows.Next() {
		var item service.ProjectResourceGroupCandidate
		if err := groupRows.Scan(&item.ID, &item.ProjectID, &item.Name, &item.Description, &item.Platform, &item.Status); err != nil {
			_ = groupRows.Close()
			return nil, err
		}
		out.Groups = append(out.Groups, item)
	}
	if err := groupRows.Close(); err != nil {
		return nil, err
	}
	if err := groupRows.Err(); err != nil {
		return nil, err
	}

	accountRows, err := r.sql.QueryContext(ctx, `
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
		WHERE deleted_at IS NULL
		  AND (
			$1 = '%%'
			OR LOWER(name) LIKE $1
			OR LOWER(COALESCE(notes, '')) LIKE $1
			OR LOWER(platform) LIKE $1
			OR LOWER(type) LIKE $1
			OR LOWER(COALESCE(credentials->>'email', credentials->>'account_email', credentials->>'user_email', extra->>'email', '')) LIKE $1
		  )
		  `+projectSearchScopeCondition(projectID, projectSQLScopeResources{AccountID: "accounts.id"})+`
		ORDER BY id ASC
		LIMIT $2
	`, like, limit)
	if err != nil {
		return nil, err
	}
	for accountRows.Next() {
		var item service.ProjectResourceAccountCandidate
		if err := accountRows.Scan(&item.ID, &item.ProjectID, &item.Name, &item.Notes, &item.Platform, &item.Type, &item.Status, &item.Email); err != nil {
			_ = accountRows.Close()
			return nil, err
		}
		out.Accounts = append(out.Accounts, item)
	}
	if err := accountRows.Close(); err != nil {
		return nil, err
	}
	if err := accountRows.Err(); err != nil {
		return nil, err
	}

	proxyRows, err := r.sql.QueryContext(ctx, `
		SELECT id, project_id, name, protocol, host, port, status
		FROM proxies
		WHERE deleted_at IS NULL
		  AND (
			$1 = '%%'
			OR LOWER(name) LIKE $1
			OR LOWER(protocol) LIKE $1
			OR LOWER(host) LIKE $1
		  )
		  `+projectSearchScopeCondition(projectID, projectSQLScopeResources{ProxyID: "proxies.id"})+`
		ORDER BY id ASC
		LIMIT $2
	`, like, limit)
	if err != nil {
		return nil, err
	}
	for proxyRows.Next() {
		var item service.ProjectResourceProxyCandidate
		if err := proxyRows.Scan(&item.ID, &item.ProjectID, &item.Name, &item.Protocol, &item.Host, &item.Port, &item.Status); err != nil {
			_ = proxyRows.Close()
			return nil, err
		}
		out.Proxies = append(out.Proxies, item)
	}
	if err := proxyRows.Close(); err != nil {
		return nil, err
	}
	if err := proxyRows.Err(); err != nil {
		return nil, err
	}

	subRows, err := r.sql.QueryContext(ctx, `
		SELECT us.id, us.user_id, us.group_id, u.email, g.name, us.status, COALESCE(us.notes, '')
		FROM user_subscriptions us
		JOIN users u ON u.id = us.user_id
		JOIN groups g ON g.id = us.group_id
		WHERE us.deleted_at IS NULL
		  AND u.deleted_at IS NULL
		  AND g.deleted_at IS NULL
		  AND (
			$1 = '%%'
			OR LOWER(u.email) LIKE $1
			OR LOWER(COALESCE(u.username, '')) LIKE $1
			OR LOWER(g.name) LIKE $1
			OR LOWER(COALESCE(us.notes, '')) LIKE $1
		  )
		  `+projectSearchScopeCondition(projectID, projectSQLScopeResources{SubscriptionID: "us.id", UserID: "us.user_id", GroupID: "us.group_id"})+`
		ORDER BY us.id ASC
		LIMIT $2
	`, like, limit)
	if err != nil {
		return nil, err
	}
	for subRows.Next() {
		var item service.ProjectResourceSubscriptionCandidate
		if err := subRows.Scan(&item.ID, &item.UserID, &item.GroupID, &item.UserEmail, &item.GroupName, &item.Status, &item.Notes); err != nil {
			_ = subRows.Close()
			return nil, err
		}
		out.Subscriptions = append(out.Subscriptions, item)
	}
	if err := subRows.Close(); err != nil {
		return nil, err
	}
	if err := subRows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func ensureProjectActiveProfile(ctx context.Context, q sqlExecutor, projectID int64) error {
	if projectID <= 0 {
		return nil
	}
	var id int64
	err := scanSingleRow(ctx, q, `
		SELECT id
		FROM project_profiles
		WHERE project_id = $1
		  AND is_active = TRUE
		  AND deleted_at IS NULL
		LIMIT 1
	`, []any{projectID}, &id)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = q.ExecContext(ctx, `
		INSERT INTO project_profiles (project_id, name, description, mode, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, TRUE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT DO NOTHING
	`, projectID, "默认配置", "Default active project application profile.", service.ProjectProfileModeRestricted)
	return err
}

func ensureProjectProfileExists(ctx context.Context, q sqlQueryer, projectID int64, profileID int64) error {
	_, err := getProjectProfileMode(ctx, q, projectID, profileID)
	return err
}

func getProjectProfileMode(ctx context.Context, q sqlQueryer, projectID int64, profileID int64) (string, error) {
	var mode string
	err := scanSingleRow(ctx, q, `
		SELECT mode
		FROM project_profiles
		WHERE project_id = $1
		  AND id = $2
		  AND deleted_at IS NULL
	`, []any{projectID, profileID}, &mode)
	if errors.Is(err, sql.ErrNoRows) {
		return "", service.ErrProjectProfileNotFound
	}
	return mode, err
}

func validateProjectBindingIDs(ctx context.Context, q sqlQueryer, input service.ProjectProfileBindingInput) error {
	checks := []struct {
		table string
		ids   []int64
		extra string
	}{
		{table: "groups", ids: input.GroupIDs, extra: "deleted_at IS NULL"},
		{table: "accounts", ids: input.AccountIDs, extra: "deleted_at IS NULL"},
		{table: "proxies", ids: input.ProxyIDs, extra: "deleted_at IS NULL"},
		{table: "user_subscriptions", ids: input.SubscriptionIDs, extra: "deleted_at IS NULL"},
	}
	for _, check := range checks {
		if len(check.ids) == 0 {
			continue
		}
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE id = ANY($1) AND %s", check.table, check.extra)
		var count int
		if err := scanSingleRow(ctx, q, query, []any{pq.Array(check.ids)}, &count); err != nil {
			return err
		}
		if count != len(check.ids) {
			return service.ErrProjectInvalidInput
		}
	}
	return nil
}

func insertProjectProfileBindings(ctx context.Context, exec sqlExecutor, profileID int64, resourceType string, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := exec.ExecContext(ctx, `
		INSERT INTO project_profile_bindings (project_profile_id, resource_type, resource_id, created_at)
		SELECT $1, $2, unnest($3::bigint[]), CURRENT_TIMESTAMP
		ON CONFLICT (project_profile_id, resource_type, resource_id) DO NOTHING
	`, profileID, resourceType, pq.Array(ids))
	return err
}

func projectSearchScopeCondition(projectID int64, resources projectSQLScopeResources) string {
	if projectID <= 0 {
		return ""
	}
	return "AND " + projectProfileScopeSQL(projectID, resources)
}

func projectSearchUserScopeCondition(projectID int64) string {
	if projectID <= 0 {
		return ""
	}
	return "AND " + projectMemberExistsSQL(projectID, "users.id")
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return strings.TrimSpace(*value)
}
