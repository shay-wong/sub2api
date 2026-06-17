package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	dbapikey "github.com/Wei-Shaw/sub2api/ent/apikey"
	dbgroup "github.com/Wei-Shaw/sub2api/ent/group"
	dbpredicate "github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func resolveProjectID(ctx context.Context, sqlq sqlExecutor) (int64, error) {
	if projectID, ok := service.ProjectIDFromContext(ctx); ok {
		return projectID, nil
	}
	return ensureDefaultProject(ctx, sqlq)
}

func resolveProjectIDForCreate(ctx context.Context, sqlq sqlExecutor, requested int64) (int64, error) {
	if projectID, ok := service.ProjectIDFromContext(ctx); ok {
		return projectID, nil
	}
	if requested > 0 {
		return requested, nil
	}
	return resolveProjectID(ctx, sqlq)
}

func projectIDForUpdate(ctx context.Context, requested int64) (int64, bool) {
	if projectID, ok := service.ProjectIDFromContext(ctx); ok {
		return projectID, true
	}
	if requested > 0 {
		return requested, true
	}
	return 0, false
}

func ensureDefaultProject(ctx context.Context, sqlq sqlExecutor) (int64, error) {
	if sqlq == nil {
		return 0, fmt.Errorf("default project lookup requires sql executor")
	}
	var projectID int64
	err := scanSingleRow(ctx, sqlq, `
		INSERT INTO projects (name, slug, description, status, profiles, created_at, updated_at)
		VALUES ($1, $2, $3, 'active', $4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (slug) DO UPDATE
		SET name = EXCLUDED.name,
			updated_at = CURRENT_TIMESTAMP
		RETURNING id
	`, []any{service.DefaultProjectName, service.DefaultProjectSlug, "Migrated default project for existing resources.", "{}"}, &projectID)
	if err == nil {
		return projectID, nil
	}
	if err == sql.ErrNoRows {
		err = scanSingleRow(ctx, sqlq, `SELECT id FROM projects WHERE slug = $1`, []any{service.DefaultProjectSlug}, &projectID)
	}
	if err != nil {
		return 0, err
	}
	return projectID, nil
}

func projectScopedAccountPredicate(ctx context.Context) []dbpredicate.Account {
	if projectID, ok := service.ProjectIDFromContext(ctx); ok {
		return []dbpredicate.Account{dbaccount.ProjectIDEQ(projectID)}
	}
	return nil
}

func projectScopedGroupPredicate(ctx context.Context) []dbpredicate.Group {
	if projectID, ok := service.ProjectIDFromContext(ctx); ok {
		return []dbpredicate.Group{dbgroup.ProjectIDEQ(projectID)}
	}
	return nil
}

func projectScopedAPIKeyPredicate(ctx context.Context) []dbpredicate.APIKey {
	if projectID, ok := service.ProjectIDFromContext(ctx); ok {
		return []dbpredicate.APIKey{dbapikey.ProjectIDEQ(projectID)}
	}
	return nil
}

func ensureDefaultProjectMember(ctx context.Context, sqlq sqlExecutor, userID int64, role string) error {
	if userID <= 0 {
		return nil
	}
	projectID, err := ensureDefaultProject(ctx, sqlq)
	if err != nil {
		return err
	}
	projectRole := service.ProjectRoleUser
	isOwner := false
	if service.RoleIsAdmin(role) {
		projectRole = service.ProjectRoleAdmin
		isOwner = service.RoleIsSuperAdmin(role)
	}
	_, err = sqlq.ExecContext(ctx, `
		INSERT INTO project_members (project_id, user_id, role, scopes, is_owner, created_at, updated_at)
		VALUES ($1, $2, $3, '[]', $4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (project_id, user_id) DO UPDATE
		SET role = EXCLUDED.role,
			is_owner = EXCLUDED.is_owner,
			updated_at = CURRENT_TIMESTAMP
	`, projectID, userID, projectRole, isOwner)
	return err
}

func appendProjectScopeWhere(ctx context.Context, conditions []string, args []any, column string) ([]string, []any) {
	projectID, ok := service.ProjectIDFromContext(ctx)
	if !ok {
		return conditions, args
	}
	column = strings.TrimSpace(column)
	if column == "" {
		column = "project_id"
	}
	conditions = append(conditions, fmt.Sprintf("%s = $%d", column, len(args)+1))
	args = append(args, projectID)
	return conditions, args
}

func appendProjectScopeQuery(ctx context.Context, query string, args []any, column string) (string, []any) {
	projectID, ok := service.ProjectIDFromContext(ctx)
	if !ok {
		return query, args
	}
	column = strings.TrimSpace(column)
	if column == "" {
		column = "project_id"
	}
	query += fmt.Sprintf(" AND %s = $%d", column, len(args)+1)
	args = append(args, projectID)
	return query, args
}
