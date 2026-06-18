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
			dbpredicate.Account(projectProfileAccountGroupBindingPredicate(projectID, dbaccount.FieldID)),
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

func projectScopedAPIKeyPredicate(ctx context.Context) []dbpredicate.APIKey {
	if projectID, ok := service.ProjectIDFromContext(ctx); ok {
		return []dbpredicate.APIKey{dbapikey.Or(
			dbpredicate.APIKey(projectProfileBindingPredicate(projectID, service.ProjectResourceTypeAPIKey, dbapikey.FieldID)),
			dbpredicate.APIKey(projectProfileBindingPredicate(projectID, service.ProjectResourceTypeUser, dbapikey.FieldUserID)),
			dbpredicate.APIKey(projectProfileBindingPredicate(projectID, service.ProjectResourceTypeGroup, dbapikey.FieldGroupID)),
			dbpredicate.APIKey(projectProfileUnrestrictedAPIKeyPredicate(projectID)),
		)}
	}
	return nil
}

func projectScopedUserPredicate(ctx context.Context) []dbpredicate.User {
	if projectID, ok := service.ProjectIDFromContext(ctx); ok {
		return []dbpredicate.User{dbuser.Or(
			dbpredicate.User(projectProfileBindingPredicate(projectID, service.ProjectResourceTypeUser, dbuser.FieldID)),
			dbpredicate.User(projectProfileUnrestrictedUserPredicate(projectID)),
		)}
	}
	return nil
}

func projectScopedUserSubscriptionPredicate(ctx context.Context) []dbpredicate.UserSubscription {
	if projectID, ok := service.ProjectIDFromContext(ctx); ok {
		return []dbpredicate.UserSubscription{dbusersubscription.Or(
			dbpredicate.UserSubscription(projectProfileBindingPredicate(projectID, service.ProjectResourceTypeSubscription, dbusersubscription.FieldID)),
			dbpredicate.UserSubscription(projectProfileBindingPredicate(projectID, service.ProjectResourceTypeUser, dbusersubscription.FieldUserID)),
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
	UserID           string
	GroupID          string
	AccountID        string
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
	resources := eventSQLScopeResources(alias)
	resources.SubscriptionID = prefixedSQLColumn(alias, "subscription_id")
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
	return eventSQLScopeResources(alias)
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
		UserID:   prefix + "user_id",
		GroupID:  prefix + "group_id",
		APIKeyID: prefix + "id",
	}
}

func projectUserScopeSQL(projectID int64, userIDColumn string) string {
	userIDColumn = strings.TrimSpace(userIDColumn)
	if userIDColumn == "" {
		return projectProfileScopeSQL(projectID, projectSQLScopeResources{})
	}
	return projectProfileScopeSQL(projectID, projectSQLScopeResources{UserID: userIDColumn})
}

func projectProfileScopeSQL(projectID int64, resources projectSQLScopeResources) string {
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
			AND %s
		)`, projectID, service.ProjectProfileModeUnrestricted, projectResourceMembershipSQL(projectID, resources)),
	}
	if c := bindingExistsSQL(projectID, service.ProjectResourceTypeUser, resources.UserID); c != "" {
		clauses = append(clauses, c)
	}
	if c := bindingExistsSQL(projectID, service.ProjectResourceTypeGroup, resources.GroupID); c != "" {
		clauses = append(clauses, c)
	}
	if c := bindingExistsSQL(projectID, service.ProjectResourceTypeAccount, resources.AccountID); c != "" {
		clauses = append(clauses, c)
	}
	if c := accountGroupBindingExistsSQL(projectID, resources.AccountID); c != "" {
		clauses = append(clauses, c)
	}
	if c := bindingExistsSQL(projectID, service.ProjectResourceTypeSubscription, resources.SubscriptionID); c != "" {
		clauses = append(clauses, c)
	}
	if c := bindingExistsSQL(projectID, service.ProjectResourceTypeAPIKey, resources.APIKeyID); c != "" {
		clauses = append(clauses, c)
	}
	scope := "(" + strings.Join(clauses, " OR ") + ")"
	if resources.RequireProjectID {
		if c := projectIDColumnSQL(projectID, resources.ProjectID); c != "" {
			return "(" + c + " AND " + scope + ")"
		}
	}
	return scope
}

func projectResourceMembershipSQL(projectID int64, resources projectSQLScopeResources) string {
	clauses := make([]string, 0, 12)
	if c := projectIDColumnSQL(projectID, resources.ProjectID); c != "" {
		clauses = append(clauses, c)
	}
	if c := projectMemberSQL(projectID, resources.UserID); c != "" {
		clauses = append(clauses, c)
	}
	if c := anyProfileBindingExistsSQL(projectID, service.ProjectResourceTypeUser, resources.UserID); c != "" {
		clauses = append(clauses, c)
	}
	if c := groupHomeProjectSQL(projectID, resources.GroupID); c != "" {
		clauses = append(clauses, c)
	}
	if c := anyProfileBindingExistsSQL(projectID, service.ProjectResourceTypeGroup, resources.GroupID); c != "" {
		clauses = append(clauses, c)
	}
	if c := accountHomeProjectSQL(projectID, resources.AccountID); c != "" {
		clauses = append(clauses, c)
	}
	if c := anyProfileBindingExistsSQL(projectID, service.ProjectResourceTypeAccount, resources.AccountID); c != "" {
		clauses = append(clauses, c)
	}
	if c := accountGroupProjectMembershipSQL(projectID, resources.AccountID); c != "" {
		clauses = append(clauses, c)
	}
	if c := anyProfileAccountGroupBindingExistsSQL(projectID, resources.AccountID); c != "" {
		clauses = append(clauses, c)
	}
	if c := subscriptionProjectMembershipSQL(projectID, resources.SubscriptionID); c != "" {
		clauses = append(clauses, c)
	}
	if c := anyProfileBindingExistsSQL(projectID, service.ProjectResourceTypeSubscription, resources.SubscriptionID); c != "" {
		clauses = append(clauses, c)
	}
	if c := apiKeyProjectMembershipSQL(projectID, resources.APIKeyID); c != "" {
		clauses = append(clauses, c)
	}
	if c := anyProfileBindingExistsSQL(projectID, service.ProjectResourceTypeAPIKey, resources.APIKeyID); c != "" {
		clauses = append(clauses, c)
	}
	if len(clauses) == 0 {
		return "FALSE"
	}
	return "(" + strings.Join(clauses, " OR ") + ")"
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

func anyProfileBindingExistsSQL(projectID int64, resourceType string, column string) string {
	column = strings.TrimSpace(column)
	if column == "" {
		return ""
	}
	return fmt.Sprintf(`EXISTS (
		SELECT 1
		FROM project_profiles pp
		JOIN project_profile_bindings ppb ON ppb.project_profile_id = pp.id
		WHERE pp.project_id = %d
		  AND pp.deleted_at IS NULL
		  AND ppb.resource_type = '%s'
		  AND ppb.resource_id = %s
	)`, projectID, resourceType, column)
}

func projectIDColumnSQL(projectID int64, column string) string {
	column = strings.TrimSpace(column)
	if column == "" {
		return ""
	}
	return fmt.Sprintf("%s = %d", column, projectID)
}

func projectMemberSQL(projectID int64, userIDColumn string) string {
	userIDColumn = strings.TrimSpace(userIDColumn)
	if userIDColumn == "" {
		return ""
	}
	return fmt.Sprintf(`EXISTS (
		SELECT 1
		FROM project_members pm
		WHERE pm.project_id = %d
		  AND pm.user_id = %s
		  AND pm.status = '%s'
	)`, projectID, userIDColumn, service.StatusActive)
}

func groupHomeProjectSQL(projectID int64, groupIDColumn string) string {
	groupIDColumn = strings.TrimSpace(groupIDColumn)
	if groupIDColumn == "" {
		return ""
	}
	return fmt.Sprintf(`EXISTS (
		SELECT 1
		FROM groups pg
		WHERE pg.id = %s
		  AND pg.project_id = %d
		  AND pg.deleted_at IS NULL
	)`, groupIDColumn, projectID)
}

func accountHomeProjectSQL(projectID int64, accountIDColumn string) string {
	accountIDColumn = strings.TrimSpace(accountIDColumn)
	if accountIDColumn == "" {
		return ""
	}
	return fmt.Sprintf(`EXISTS (
		SELECT 1
		FROM accounts account_home
		WHERE account_home.id = %s
		  AND account_home.project_id = %d
		  AND account_home.deleted_at IS NULL
	)`, accountIDColumn, projectID)
}

func accountGroupProjectMembershipSQL(projectID int64, accountIDColumn string) string {
	accountIDColumn = strings.TrimSpace(accountIDColumn)
	if accountIDColumn == "" {
		return ""
	}
	return fmt.Sprintf(`EXISTS (
		SELECT 1
		FROM account_groups pag
		JOIN groups pg ON pg.id = pag.group_id
		WHERE pag.account_id = %s
		  AND pg.project_id = %d
		  AND pg.deleted_at IS NULL
	)`, accountIDColumn, projectID)
}

func subscriptionProjectMembershipSQL(projectID int64, subscriptionIDColumn string) string {
	subscriptionIDColumn = strings.TrimSpace(subscriptionIDColumn)
	if subscriptionIDColumn == "" {
		return ""
	}
	return fmt.Sprintf(`EXISTS (
		SELECT 1
		FROM user_subscriptions pus
		LEFT JOIN project_members ppm ON ppm.project_id = %d AND ppm.user_id = pus.user_id AND ppm.status = '%s'
		LEFT JOIN groups pg ON pg.id = pus.group_id AND pg.project_id = %d AND pg.deleted_at IS NULL
		LEFT JOIN project_profiles pp ON pp.project_id = %d AND pp.deleted_at IS NULL
		LEFT JOIN project_profile_bindings ppb_user ON ppb_user.project_profile_id = pp.id AND ppb_user.resource_type = '%s' AND ppb_user.resource_id = pus.user_id
		LEFT JOIN project_profile_bindings ppb_group ON ppb_group.project_profile_id = pp.id AND ppb_group.resource_type = '%s' AND ppb_group.resource_id = pus.group_id
		WHERE pus.id = %s
		  AND pus.deleted_at IS NULL
		  AND (ppm.user_id IS NOT NULL OR pg.id IS NOT NULL OR ppb_user.id IS NOT NULL OR ppb_group.id IS NOT NULL)
	)`, projectID, service.StatusActive, projectID, projectID, service.ProjectResourceTypeUser, service.ProjectResourceTypeGroup, subscriptionIDColumn)
}

func apiKeyProjectMembershipSQL(projectID int64, apiKeyIDColumn string) string {
	apiKeyIDColumn = strings.TrimSpace(apiKeyIDColumn)
	if apiKeyIDColumn == "" {
		return ""
	}
	return fmt.Sprintf(`EXISTS (
		SELECT 1
		FROM api_keys pak
		LEFT JOIN project_members ppm ON ppm.project_id = %d AND ppm.user_id = pak.user_id AND ppm.status = '%s'
		LEFT JOIN groups pg ON pg.id = pak.group_id AND pg.project_id = %d AND pg.deleted_at IS NULL
		LEFT JOIN project_profiles pp ON pp.project_id = %d AND pp.deleted_at IS NULL
		LEFT JOIN project_profile_bindings ppb_user ON ppb_user.project_profile_id = pp.id AND ppb_user.resource_type = '%s' AND ppb_user.resource_id = pak.user_id
		LEFT JOIN project_profile_bindings ppb_group ON ppb_group.project_profile_id = pp.id AND ppb_group.resource_type = '%s' AND ppb_group.resource_id = pak.group_id
		WHERE pak.id = %s
		  AND pak.deleted_at IS NULL
		  AND (pak.project_id = %d OR ppm.user_id IS NOT NULL OR pg.id IS NOT NULL OR ppb_user.id IS NOT NULL OR ppb_group.id IS NOT NULL)
	)`, projectID, service.StatusActive, projectID, projectID, service.ProjectResourceTypeUser, service.ProjectResourceTypeGroup, apiKeyIDColumn, projectID)
}

func accountGroupBindingExistsSQL(projectID int64, accountIDColumn string) string {
	accountIDColumn = strings.TrimSpace(accountIDColumn)
	if accountIDColumn == "" {
		return ""
	}
	return fmt.Sprintf(`EXISTS (
		SELECT 1
		FROM account_groups ag
		JOIN project_profiles pp ON pp.project_id = %d
		JOIN project_profile_bindings ppb ON ppb.project_profile_id = pp.id
		WHERE ag.account_id = %s
		  AND pp.is_active = TRUE
		  AND pp.deleted_at IS NULL
		  AND pp.mode = '%s'
		  AND ppb.resource_type = '%s'
		  AND ppb.resource_id = ag.group_id
	)`, projectID, accountIDColumn, service.ProjectProfileModeRestricted, service.ProjectResourceTypeGroup)
}

func anyProfileAccountGroupBindingExistsSQL(projectID int64, accountIDColumn string) string {
	accountIDColumn = strings.TrimSpace(accountIDColumn)
	if accountIDColumn == "" {
		return ""
	}
	return fmt.Sprintf(`EXISTS (
		SELECT 1
		FROM account_groups ag
		JOIN project_profiles pp ON pp.project_id = %d
		JOIN project_profile_bindings ppb ON ppb.project_profile_id = pp.id
		WHERE ag.account_id = %s
		  AND pp.deleted_at IS NULL
		  AND ppb.resource_type = '%s'
		  AND ppb.resource_id = ag.group_id
	)`, projectID, accountIDColumn, service.ProjectResourceTypeGroup)
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

func projectProfileAccountGroupBindingPredicate(projectID int64, accountIDField string) func(*entsql.Selector) {
	return func(s *entsql.Selector) {
		pp := entsql.Table("project_profiles")
		pb := entsql.Table("project_profile_bindings")
		ag := entsql.Table("account_groups")
		s.Where(entsql.Exists(
			entsql.SelectExpr(entsql.Expr("1")).
				From(pp).
				Join(pb).
				On(pp.C("id"), pb.C("project_profile_id")).
				Join(ag).
				On(pb.C("resource_id"), ag.C("group_id")).
				Where(entsql.And(
					entsql.EQ(pp.C("project_id"), projectID),
					entsql.EQ(pp.C("is_active"), true),
					entsql.IsNull(pp.C("deleted_at")),
					entsql.EQ(pp.C("mode"), service.ProjectProfileModeRestricted),
					entsql.EQ(pb.C("resource_type"), service.ProjectResourceTypeGroup),
					entsql.ColumnsEQ(ag.C("account_id"), s.C(accountIDField)),
				)),
		))
	}
}

func projectProfileUnrestrictedUserPredicate(projectID int64) func(*entsql.Selector) {
	return func(s *entsql.Selector) {
		s.Where(entsql.And(
			projectProfileUnrestrictedExists(projectID),
			entsql.Or(
				projectMemberExistsPredicate(projectID, s.C(dbuser.FieldID)),
				projectProfileAnyBindingExistsPredicate(projectID, service.ProjectResourceTypeUser, s.C(dbuser.FieldID)),
			),
		))
	}
}

func projectProfileUnrestrictedGroupPredicate(projectID int64) func(*entsql.Selector) {
	return func(s *entsql.Selector) {
		s.Where(entsql.And(
			projectProfileUnrestrictedExists(projectID),
			entsql.Or(
				entsql.EQ(s.C(dbgroup.FieldProjectID), projectID),
				projectProfileAnyBindingExistsPredicate(projectID, service.ProjectResourceTypeGroup, s.C(dbgroup.FieldID)),
			),
		))
	}
}

func projectProfileUnrestrictedAccountPredicate(projectID int64) func(*entsql.Selector) {
	return func(s *entsql.Selector) {
		s.Where(entsql.And(
			projectProfileUnrestrictedExists(projectID),
			entsql.Or(
				entsql.EQ(s.C(dbaccount.FieldProjectID), projectID),
				projectProfileAnyBindingExistsPredicate(projectID, service.ProjectResourceTypeAccount, s.C(dbaccount.FieldID)),
				accountGroupProjectMembershipPredicate(projectID, s.C(dbaccount.FieldID)),
				projectProfileAnyAccountGroupBindingExistsPredicate(projectID, s.C(dbaccount.FieldID)),
			),
		))
	}
}

func projectProfileUnrestrictedAPIKeyPredicate(projectID int64) func(*entsql.Selector) {
	return func(s *entsql.Selector) {
		s.Where(entsql.And(
			projectProfileUnrestrictedExists(projectID),
			entsql.Or(
				entsql.EQ(s.C(dbapikey.FieldProjectID), projectID),
				projectProfileAnyBindingExistsPredicate(projectID, service.ProjectResourceTypeAPIKey, s.C(dbapikey.FieldID)),
				projectMemberExistsPredicate(projectID, s.C(dbapikey.FieldUserID)),
				projectProfileAnyBindingExistsPredicate(projectID, service.ProjectResourceTypeUser, s.C(dbapikey.FieldUserID)),
				groupHomeProjectPredicate(projectID, s.C(dbapikey.FieldGroupID)),
				projectProfileAnyBindingExistsPredicate(projectID, service.ProjectResourceTypeGroup, s.C(dbapikey.FieldGroupID)),
			),
		))
	}
}

func projectProfileUnrestrictedUserSubscriptionPredicate(projectID int64) func(*entsql.Selector) {
	return func(s *entsql.Selector) {
		s.Where(entsql.And(
			projectProfileUnrestrictedExists(projectID),
			entsql.Or(
				projectProfileAnyBindingExistsPredicate(projectID, service.ProjectResourceTypeSubscription, s.C(dbusersubscription.FieldID)),
				projectMemberExistsPredicate(projectID, s.C(dbusersubscription.FieldUserID)),
				projectProfileAnyBindingExistsPredicate(projectID, service.ProjectResourceTypeUser, s.C(dbusersubscription.FieldUserID)),
				groupHomeProjectPredicate(projectID, s.C(dbusersubscription.FieldGroupID)),
				projectProfileAnyBindingExistsPredicate(projectID, service.ProjectResourceTypeGroup, s.C(dbusersubscription.FieldGroupID)),
			),
		))
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

func projectProfileAnyBindingExistsPredicate(projectID int64, resourceType string, resourceIDColumn string) *entsql.Predicate {
	return entsql.P(func(b *entsql.Builder) {
		b.WriteString("EXISTS (SELECT 1 FROM project_profiles pp JOIN project_profile_bindings ppb ON ppb.project_profile_id = pp.id WHERE pp.project_id = ").
			Arg(projectID).
			WriteString(" AND pp.deleted_at IS NULL AND ppb.resource_type = ").
			Arg(resourceType).
			WriteString(" AND ppb.resource_id = ").
			WriteString(resourceIDColumn).
			WriteString(")")
	})
}

func projectMemberExistsPredicate(projectID int64, userIDColumn string) *entsql.Predicate {
	return entsql.P(func(b *entsql.Builder) {
		b.WriteString("EXISTS (SELECT 1 FROM project_members pm WHERE pm.project_id = ").
			Arg(projectID).
			WriteString(" AND pm.user_id = ").
			WriteString(userIDColumn).
			WriteString(" AND pm.status = ").
			Arg(service.StatusActive).
			WriteString(")")
	})
}

func groupHomeProjectPredicate(projectID int64, groupIDColumn string) *entsql.Predicate {
	return entsql.P(func(b *entsql.Builder) {
		b.WriteString("EXISTS (SELECT 1 FROM groups pg WHERE pg.id = ").
			WriteString(groupIDColumn).
			WriteString(" AND pg.project_id = ").
			Arg(projectID).
			WriteString(" AND pg.deleted_at IS NULL)")
	})
}

func accountGroupProjectMembershipPredicate(projectID int64, accountIDColumn string) *entsql.Predicate {
	return entsql.P(func(b *entsql.Builder) {
		b.WriteString("EXISTS (SELECT 1 FROM account_groups pag JOIN groups pg ON pg.id = pag.group_id WHERE pag.account_id = ").
			WriteString(accountIDColumn).
			WriteString(" AND pg.project_id = ").
			Arg(projectID).
			WriteString(" AND pg.deleted_at IS NULL)")
	})
}

func projectProfileAnyAccountGroupBindingExistsPredicate(projectID int64, accountIDColumn string) *entsql.Predicate {
	return entsql.P(func(b *entsql.Builder) {
		b.WriteString("EXISTS (SELECT 1 FROM account_groups pag JOIN project_profiles pp ON pp.project_id = ").
			Arg(projectID).
			WriteString(" JOIN project_profile_bindings ppb ON ppb.project_profile_id = pp.id WHERE pag.account_id = ").
			WriteString(accountIDColumn).
			WriteString(" AND pp.deleted_at IS NULL AND ppb.resource_type = ").
			Arg(service.ProjectResourceTypeGroup).
			WriteString(" AND ppb.resource_id = pag.group_id)")
	})
}
