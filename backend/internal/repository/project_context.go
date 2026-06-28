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
	dbproxy "github.com/Wei-Shaw/sub2api/ent/proxy"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	dbusersubscription "github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/service"

	entsql "entgo.io/ent/dialect/sql"
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
		_ = ensureProjectActiveProfile(ctx, sqlq, projectID)
		return projectID, nil
	}
	if err == sql.ErrNoRows {
		err = scanSingleRow(ctx, sqlq, `SELECT id FROM projects WHERE slug = $1`, []any{service.DefaultProjectSlug}, &projectID)
	}
	if err != nil {
		return 0, err
	}
	_ = ensureProjectActiveProfile(ctx, sqlq, projectID)
	return projectID, nil
}

func projectScopedAccountPredicate(ctx context.Context) []dbpredicate.Account {
	if projectID, ok := service.ProjectIDFromContext(ctx); ok {
		return []dbpredicate.Account{dbaccount.Or(
			dbpredicate.Account(projectProfileBindingPredicate(projectID, service.ProjectResourceTypeAccount, dbaccount.FieldID)),
			dbpredicate.Account(projectProfileUnrestrictedAccountPredicate(projectID)),
		)}
	}
	return nil
}

func projectScopedGroupPredicate(ctx context.Context) []dbpredicate.Group {
	if projectID, ok := service.ProjectIDFromContext(ctx); ok {
		return []dbpredicate.Group{dbgroup.Or(
			dbpredicate.Group(projectProfileBindingPredicate(projectID, service.ProjectResourceTypeGroup, dbgroup.FieldID)),
			dbpredicate.Group(projectProfileUnrestrictedGroupPredicate(projectID)),
		)}
	}
	return nil
}

func projectScopedProxyPredicate(ctx context.Context) []dbpredicate.Proxy {
	if projectID, ok := service.ProjectIDFromContext(ctx); ok {
		return []dbpredicate.Proxy{dbproxy.Or(
			dbpredicate.Proxy(projectProfileBindingPredicate(projectID, service.ProjectResourceTypeProxy, dbproxy.FieldID)),
			dbpredicate.Proxy(projectProfileUnrestrictedProxyPredicate(projectID)),
		)}
	}
	return nil
}

func projectScopedAPIKeyPredicate(ctx context.Context) []dbpredicate.APIKey {
	if projectID, ok := service.ProjectIDFromContext(ctx); ok {
		memberPredicate := projectMemberExistsPredicate
		if service.RequireActiveProjectMemberFromContext(ctx) {
			memberPredicate = projectMemberBindingPredicate
		}
		return []dbpredicate.APIKey{dbapikey.And(
			dbapikey.ProjectIDEQ(projectID),
			dbpredicate.APIKey(memberPredicate(projectID, dbapikey.FieldUserID)),
		)}
	}
	return nil
}

func projectScopedUserPredicate(ctx context.Context) []dbpredicate.User {
	if projectID, ok := service.ProjectIDFromContext(ctx); ok {
		return []dbpredicate.User{dbpredicate.User(projectMemberExistsPredicate(projectID, dbuser.FieldID))}
	}
	return nil
}

func projectScopedUserSubscriptionPredicate(ctx context.Context) []dbpredicate.UserSubscription {
	if projectID, ok := service.ProjectIDFromContext(ctx); ok {
		return []dbpredicate.UserSubscription{dbusersubscription.Or(
			dbpredicate.UserSubscription(projectProfileBindingPredicate(projectID, service.ProjectResourceTypeSubscription, dbusersubscription.FieldID)),
			dbpredicate.UserSubscription(projectProfileBindingPredicate(projectID, service.ProjectResourceTypeGroup, dbusersubscription.FieldGroupID)),
			dbpredicate.UserSubscription(projectProfileUnrestrictedUserSubscriptionPredicate(projectID)),
		)}
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
	return ensureProjectMember(ctx, sqlq, projectID, userID, role, service.RoleIsSuperAdmin(role))
}

func ensureProjectMember(ctx context.Context, sqlq sqlExecutor, projectID int64, userID int64, role string, isOwner bool) error {
	if sqlq == nil || projectID <= 0 || userID <= 0 {
		return nil
	}
	projectRole := service.ProjectRoleUser
	if service.RoleIsAdmin(role) {
		projectRole = service.ProjectRoleAdmin
	}
	_, err := sqlq.ExecContext(ctx, `
		INSERT INTO project_members (project_id, user_id, role, scopes, is_owner, status, created_at, updated_at)
		VALUES ($1, $2, $3, '[]', $4, $5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (project_id, user_id) DO UPDATE
		SET role = EXCLUDED.role,
			is_owner = EXCLUDED.is_owner,
			status = EXCLUDED.status,
			updated_at = CURRENT_TIMESTAMP
	`, projectID, userID, projectRole, isOwner, service.StatusActive)
	return err
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

func appendProjectProfileScopedQuery(ctx context.Context, query string, args []any, column string, resources projectSQLScopeResources) (string, []any) {
	projectID, ok := service.ProjectIDFromContext(ctx)
	if !ok {
		return query, args
	}
	resources = withProjectIDColumn(resources, column)
	query += " AND " + projectProfileScopeSQL(projectID, resources)
	return query, args
}

func appendProjectProfileScopedWhere(ctx context.Context, conditions []string, args []any, column string, resources projectSQLScopeResources) ([]string, []any) {
	projectID, ok := service.ProjectIDFromContext(ctx)
	if !ok {
		return conditions, args
	}
	resources = withProjectIDColumn(resources, column)
	conditions = append(conditions, projectProfileScopeSQL(projectID, resources))
	return conditions, args
}

func appendProjectProfileScopedWhereAt(ctx context.Context, conditions []string, args []any, nextIndex int, column string, resources projectSQLScopeResources) ([]string, []any, int) {
	projectID, ok := service.ProjectIDFromContext(ctx)
	if !ok {
		return conditions, args, nextIndex
	}
	resources = withProjectIDColumn(resources, column)
	conditions = append(conditions, projectProfileScopeSQL(projectID, resources))
	return conditions, args, nextIndex
}

func buildProjectProfileScopedClause(ctx context.Context, args *[]any, column string, resources projectSQLScopeResources) string {
	projectID, ok := service.ProjectIDFromContext(ctx)
	if !ok {
		return ""
	}
	if args == nil {
		return ""
	}
	resources = withProjectIDColumn(resources, column)
	return " AND " + projectProfileScopeSQL(projectID, resources)
}

type projectSQLScopeResources struct {
	ProjectID        string
	RequireProjectID bool
	RequireMember    bool
	APIKeyResource   bool
	UserID           string
	GroupID          string
	AccountID        string
	ProxyID          string
	SubscriptionID   string
	APIKeyID         string
}

func withProjectIDColumn(resources projectSQLScopeResources, column string) projectSQLScopeResources {
	column = strings.TrimSpace(column)
	if resources.ProjectID == "" && column != "" {
		resources.ProjectID = column
	}
	return resources
}

func usageLogSQLScopeResources(alias string) projectSQLScopeResources {
	if strings.TrimSpace(alias) == "" {
		alias = "usage_logs"
	}
	resources := eventSQLScopeResources(alias)
	resources.RequireProjectID = false
	resources.SubscriptionID = prefixedSQLColumn(alias, "subscription_id")
	resources.RequireMember = true
	return resources
}

func eventSQLScopeResources(alias string) projectSQLScopeResources {
	prefix := strings.TrimSpace(alias)
	if prefix != "" {
		prefix += "."
	}
	return projectSQLScopeResources{
		ProjectID:        prefix + "project_id",
		RequireProjectID: true,
		UserID:           prefix + "user_id",
		GroupID:          prefix + "group_id",
		AccountID:        prefix + "account_id",
		APIKeyID:         prefix + "api_key_id",
	}
}

func opsErrorSQLScopeResources(alias string) projectSQLScopeResources {
	if strings.TrimSpace(alias) == "" {
		alias = "ops_error_logs"
	}
	resources := eventSQLScopeResources(alias)
	resources.RequireProjectID = false
	resources.RequireMember = true
	return resources
}

func prefixedSQLColumn(alias string, column string) string {
	prefix := strings.TrimSpace(alias)
	if prefix != "" {
		prefix += "."
	}
	return prefix + column
}

func apiKeySQLScopeResources(alias string) projectSQLScopeResources {
	prefix := strings.TrimSpace(alias)
	if prefix != "" {
		prefix += "."
	}
	return projectSQLScopeResources{
		ProjectID:        prefix + "project_id",
		RequireProjectID: true,
		APIKeyResource:   true,
		UserID:           prefix + "user_id",
	}
}

func projectUserScopeSQL(projectID int64, userIDColumn string) string {
	userIDColumn = strings.TrimSpace(userIDColumn)
	if userIDColumn == "" {
		return "FALSE"
	}
	return projectMemberExistsSQL(projectID, userIDColumn)
}

func projectProfileScopeSQL(projectID int64, resources projectSQLScopeResources) string {
	if resources.APIKeyResource {
		return apiKeyProjectScopeSQL(projectID, resources)
	}
	if resources.UserID != "" && resources.GroupID == "" && resources.AccountID == "" && resources.ProxyID == "" && resources.SubscriptionID == "" && resources.APIKeyID == "" {
		return projectUserScopeSQL(projectID, resources.UserID)
	}
	clauses := []string{
		fmt.Sprintf(`(
			EXISTS (
			SELECT 1
			FROM project_profiles pp
			WHERE pp.project_id = %d
			  AND pp.is_active = TRUE
			  AND pp.deleted_at IS NULL
			  AND pp.mode = '%s'
			)
		)`, projectID, service.ProjectProfileModeUnrestricted),
	}
	if c := bindingExistsSQL(projectID, service.ProjectResourceTypeGroup, resources.GroupID); c != "" {
		clauses = append(clauses, c)
	}
	if c := bindingExistsSQL(projectID, service.ProjectResourceTypeAccount, resources.AccountID); c != "" {
		clauses = append(clauses, c)
	}
	if c := bindingExistsSQL(projectID, service.ProjectResourceTypeProxy, resources.ProxyID); c != "" {
		clauses = append(clauses, c)
	}
	if c := bindingExistsSQL(projectID, service.ProjectResourceTypeSubscription, resources.SubscriptionID); c != "" {
		clauses = append(clauses, c)
	}
	scope := "(" + strings.Join(clauses, " OR ") + ")"
	if resources.RequireProjectID {
		if c := projectIDColumnSQL(projectID, resources.ProjectID); c != "" {
			scope = "(" + c + " AND " + scope + ")"
		}
	}
	if resources.RequireMember {
		if c := projectMemberExistsSQL(projectID, resources.UserID); c != "" {
			scope = "(" + c + " AND " + scope + ")"
		}
	}
	return scope
}

func apiKeyProjectScopeSQL(projectID int64, resources projectSQLScopeResources) string {
	projectClause := projectIDColumnSQL(projectID, resources.ProjectID)
	memberClause := projectMemberExistsSQL(projectID, resources.UserID)
	if projectClause == "" || memberClause == "" {
		return "FALSE"
	}
	return "(" + projectClause + " AND " + memberClause + ")"
}

func projectUserGroupScopeSQL(projectID int64, userIDColumn string, groupIDColumn string) string {
	userClause := projectUserScopeSQL(projectID, userIDColumn)
	groupClause := projectProfileScopeSQL(projectID, projectSQLScopeResources{GroupID: groupIDColumn})
	if userClause == "FALSE" || groupClause == "FALSE" {
		return "FALSE"
	}
	return "(" + userClause + " AND " + groupClause + ")"
}

func bindResourceToActiveProjectProfile(ctx context.Context, sqlq sqlExecutor, projectID int64, resourceType string, resourceID int64) error {
	if sqlq == nil || projectID <= 0 || resourceID <= 0 || strings.TrimSpace(resourceType) == "" {
		return nil
	}
	if err := ensureProjectActiveProfile(ctx, sqlq, projectID); err != nil {
		return err
	}
	_, err := sqlq.ExecContext(ctx, `
		INSERT INTO project_profile_bindings (project_profile_id, resource_type, resource_id, created_at, metadata)
		SELECT pp.id, $2, $3, CURRENT_TIMESTAMP, $5
		FROM project_profiles pp
		WHERE pp.project_id = $1
		  AND pp.is_active = TRUE
		  AND pp.deleted_at IS NULL
		  AND pp.mode = $4
		ON CONFLICT (project_profile_id, resource_type, resource_id) DO NOTHING
	`, projectID, resourceType, resourceID, service.ProjectProfileModeRestricted, "{}")
	return err
}

func bindingExistsSQL(projectID int64, resourceType string, column string) string {
	column = strings.TrimSpace(column)
	if column == "" {
		return ""
	}
	return fmt.Sprintf(`EXISTS (
		SELECT 1
		FROM project_profiles pp
		JOIN project_profile_bindings ppb ON ppb.project_profile_id = pp.id
		WHERE pp.project_id = %d
		  AND pp.is_active = TRUE
		  AND pp.deleted_at IS NULL
		  AND pp.mode = '%s'
		  AND ppb.resource_type = '%s'
		  AND ppb.resource_id = %s
	)`, projectID, service.ProjectProfileModeRestricted, resourceType, column)
}

func projectIDColumnSQL(projectID int64, column string) string {
	column = strings.TrimSpace(column)
	if column == "" {
		return ""
	}
	return fmt.Sprintf("%s = %d", column, projectID)
}

func projectMemberExistsSQL(projectID int64, userIDColumn string) string {
	return projectMemberSQLWithStatus(projectID, userIDColumn, false)
}

func projectMemberSQLWithStatus(projectID int64, userIDColumn string, activeOnly bool) string {
	userIDColumn = strings.TrimSpace(userIDColumn)
	if userIDColumn == "" {
		return ""
	}
	statusClause := ""
	if activeOnly {
		statusClause = fmt.Sprintf(`
		  AND pm.status = '%s'`, service.StatusActive)
	}
	return fmt.Sprintf(`EXISTS (
		SELECT 1
		FROM project_members pm
		WHERE pm.project_id = %d
		  AND pm.user_id = %s%s
	)`, projectID, userIDColumn, statusClause)
}

func projectProfileBindingPredicate(projectID int64, resourceType string, resourceIDField string) func(*entsql.Selector) {
	return func(s *entsql.Selector) {
		pp := entsql.Table("project_profiles")
		pb := entsql.Table("project_profile_bindings")
		s.Where(entsql.Exists(
			entsql.SelectExpr(entsql.Expr("1")).
				From(pp).
				Join(pb).
				On(pp.C("id"), pb.C("project_profile_id")).
				Where(entsql.And(
					entsql.EQ(pp.C("project_id"), projectID),
					entsql.EQ(pp.C("is_active"), true),
					entsql.IsNull(pp.C("deleted_at")),
					entsql.EQ(pp.C("mode"), service.ProjectProfileModeRestricted),
					entsql.EQ(pb.C("resource_type"), resourceType),
					entsql.ColumnsEQ(pb.C("resource_id"), s.C(resourceIDField)),
				)),
		))
	}
}

func projectMemberBindingPredicate(projectID int64, userIDField string) func(*entsql.Selector) {
	return projectMemberPredicate(projectID, userIDField, true)
}

func projectMemberExistsPredicate(projectID int64, userIDField string) func(*entsql.Selector) {
	return projectMemberPredicate(projectID, userIDField, false)
}

func projectMemberPredicate(projectID int64, userIDField string, activeOnly bool) func(*entsql.Selector) {
	return func(s *entsql.Selector) {
		pm := entsql.Table("project_members")
		conditions := []*entsql.Predicate{
			entsql.EQ(pm.C("project_id"), projectID),
			entsql.ColumnsEQ(pm.C("user_id"), s.C(userIDField)),
		}
		if activeOnly {
			conditions = append(conditions, entsql.EQ(pm.C("status"), service.StatusActive))
		}
		s.Where(entsql.Exists(
			entsql.SelectExpr(entsql.Expr("1")).
				From(pm).
				Where(entsql.And(conditions...)),
		))
	}
}

func projectProfileUnrestrictedGroupPredicate(projectID int64) func(*entsql.Selector) {
	return func(s *entsql.Selector) {
		s.Where(projectProfileUnrestrictedExists(projectID))
	}
}

func projectProfileUnrestrictedAccountPredicate(projectID int64) func(*entsql.Selector) {
	return func(s *entsql.Selector) {
		s.Where(projectProfileUnrestrictedExists(projectID))
	}
}

func projectProfileUnrestrictedProxyPredicate(projectID int64) func(*entsql.Selector) {
	return func(s *entsql.Selector) {
		s.Where(projectProfileUnrestrictedExists(projectID))
	}
}

func projectProfileUnrestrictedUserSubscriptionPredicate(projectID int64) func(*entsql.Selector) {
	return func(s *entsql.Selector) {
		s.Where(projectProfileUnrestrictedExists(projectID))
	}
}

func projectProfileUnrestrictedExists(projectID int64) *entsql.Predicate {
	return entsql.P(func(b *entsql.Builder) {
		b.WriteString("EXISTS (SELECT 1 FROM project_profiles pp WHERE pp.project_id = ").
			Arg(projectID).
			WriteString(" AND pp.is_active = TRUE AND pp.deleted_at IS NULL AND pp.mode = ").
			Arg(service.ProjectProfileModeUnrestricted).
			WriteString(")")
	})
}
