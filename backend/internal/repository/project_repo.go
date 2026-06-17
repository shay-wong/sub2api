package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

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
		SELECT p.id, p.name, p.slug, p.description, pm.role, pm.is_owner
		FROM project_members pm
		JOIN projects p ON p.id = pm.project_id
		WHERE pm.user_id = $1
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
		if scanErr := rows.Scan(&item.ID, &item.Name, &item.Slug, &description, &item.Role, &item.IsOwner); scanErr != nil {
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
			pm.is_owner,
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
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&item.ProjectID, &item.UserID, &item.Email, &item.Username, &item.Role, &item.IsOwner, &item.Status, &createdAt, &updatedAt); err != nil {
			return nil, err
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

func (r *projectRepository) SetProjectMember(ctx context.Context, projectID int64, input service.ProjectMemberInput) (*service.ProjectMember, error) {
	if r == nil || r.sql == nil {
		return nil, fmt.Errorf("nil project repository")
	}
	var exists int64
	if err := scanSingleRow(ctx, r.sql, `
		SELECT id FROM users WHERE id = $1 AND deleted_at IS NULL LIMIT 1
	`, []any{input.UserID}, &exists); errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrPermissionUserNotFound
	} else if err != nil {
		return nil, err
	}

	var item service.ProjectMember
	var createdAt, updatedAt time.Time
	err := scanSingleRow(ctx, r.sql, `
		WITH upserted AS (
			INSERT INTO project_members (project_id, user_id, role, scopes, is_owner, created_at, updated_at)
			VALUES ($1, $2, $3, '[]', $4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			ON CONFLICT (project_id, user_id) DO UPDATE
			SET role = EXCLUDED.role,
				is_owner = EXCLUDED.is_owner,
				updated_at = CURRENT_TIMESTAMP
			RETURNING project_id, user_id, role, is_owner, created_at, updated_at
		)
		SELECT
			up.project_id,
			up.user_id,
			u.email,
			COALESCE(u.username, ''),
			up.role,
			up.is_owner,
			u.status,
			up.created_at,
			up.updated_at
		FROM upserted up
		JOIN users u ON u.id = up.user_id
	`, []any{projectID, input.UserID, input.Role, input.IsOwner}, &item.ProjectID, &item.UserID, &item.Email, &item.Username, &item.Role, &item.IsOwner, &item.Status, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	return &item, nil
}

func (r *projectRepository) RemoveProjectMember(ctx context.Context, projectID int64, userID int64) error {
	if r == nil || r.sql == nil {
		return fmt.Errorf("nil project repository")
	}
	_, err := r.sql.ExecContext(ctx, `
		DELETE FROM project_members
		WHERE project_id = $1
		  AND user_id = $2
	`, projectID, userID)
	return err
}

func (r *projectRepository) MoveProjectResources(ctx context.Context, projectID int64, input service.ProjectResourceMoveInput) (*service.ProjectResourceMoveResult, error) {
	if r == nil || r.sql == nil {
		return nil, fmt.Errorf("nil project repository")
	}
	tx, err := beginSQLTx(ctx, r.sql)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	result := &service.ProjectResourceMoveResult{}
	accountIDs := input.AccountIDs
	apiKeyIDs := input.APIKeyIDs
	groupIDs := input.GroupIDs

	if err := loadProjectMoveInvalidationScope(ctx, tx, accountIDs, apiKeyIDs, groupIDs, result); err != nil {
		return nil, err
	}

	if len(groupIDs) > 0 {
		n, err := execRowsAffected(ctx, tx, `
			UPDATE groups
			SET project_id = $2,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ANY($1)
			  AND deleted_at IS NULL
			  AND project_id <> $2
		`, pq.Array(groupIDs), projectID)
		if err != nil {
			return nil, err
		}
		result.GroupsMoved = n
	}

	if len(accountIDs) > 0 {
		n, err := execRowsAffected(ctx, tx, `
			UPDATE accounts
			SET project_id = $2,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ANY($1)
			  AND deleted_at IS NULL
			  AND project_id <> $2
		`, pq.Array(accountIDs), projectID)
		if err != nil {
			return nil, err
		}
		result.AccountsMoved = n
	}

	if len(apiKeyIDs) > 0 {
		n, err := execRowsAffected(ctx, tx, `
			UPDATE api_keys
			SET project_id = $2,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ANY($1)
			  AND deleted_at IS NULL
			  AND project_id <> $2
		`, pq.Array(apiKeyIDs), projectID)
		if err != nil {
			return nil, err
		}
		result.APIKeysMoved = n
	}

	if len(groupIDs) > 0 {
		n, err := execRowsAffected(ctx, tx, `
			UPDATE groups g
			SET fallback_group_id = NULL,
				updated_at = CURRENT_TIMESTAMP
			WHERE g.id = ANY($1)
			  AND g.fallback_group_id IS NOT NULL
			  AND NOT EXISTS (
				SELECT 1
				FROM groups fg
				WHERE fg.id = g.fallback_group_id
				  AND fg.project_id = g.project_id
				  AND fg.deleted_at IS NULL
			  )
		`, pq.Array(groupIDs))
		if err != nil {
			return nil, err
		}
		result.GroupFallbacksCleared += n

		n, err = execRowsAffected(ctx, tx, `
			UPDATE groups g
			SET fallback_group_id_on_invalid_request = NULL,
				updated_at = CURRENT_TIMESTAMP
			WHERE g.id = ANY($1)
			  AND g.fallback_group_id_on_invalid_request IS NOT NULL
			  AND NOT EXISTS (
				SELECT 1
				FROM groups fg
				WHERE fg.id = g.fallback_group_id_on_invalid_request
				  AND fg.project_id = g.project_id
				  AND fg.deleted_at IS NULL
			  )
		`, pq.Array(groupIDs))
		if err != nil {
			return nil, err
		}
		result.GroupFallbacksCleared += n

		n, err = execRowsAffected(ctx, tx, `
			WITH filtered AS (
				SELECT
					g.id,
					COALESCE(
						jsonb_object_agg(route.key, route.valid_ids) FILTER (WHERE jsonb_array_length(route.valid_ids) > 0),
						'{}'::jsonb
					) AS routing
				FROM groups g
				CROSS JOIN LATERAL (
					SELECT
						entry.key,
						COALESCE(jsonb_agg(elem.value ORDER BY elem.ord), '[]'::jsonb) AS valid_ids
					FROM jsonb_each(g.model_routing) AS entry(key, value)
					LEFT JOIN LATERAL jsonb_array_elements(
						CASE
							WHEN jsonb_typeof(entry.value) = 'array' THEN entry.value
							ELSE '[]'::jsonb
						END
					) WITH ORDINALITY AS elem(value, ord)
						ON jsonb_typeof(elem.value) = 'number'
					LEFT JOIN accounts a
						ON a.id = (elem.value #>> '{}')::bigint
						AND a.project_id = g.project_id
						AND a.deleted_at IS NULL
					WHERE a.id IS NOT NULL
					GROUP BY entry.key
				) route
				WHERE g.id = ANY($1)
				  AND g.model_routing <> '{}'::jsonb
				GROUP BY g.id
			)
			UPDATE groups g
			SET model_routing = filtered.routing,
				model_routing_enabled = CASE WHEN filtered.routing = '{}'::jsonb THEN FALSE ELSE g.model_routing_enabled END,
				updated_at = CURRENT_TIMESTAMP
			FROM filtered
			WHERE g.id = filtered.id
			  AND (
				g.model_routing <> filtered.routing
				OR (filtered.routing = '{}'::jsonb AND g.model_routing_enabled = TRUE)
			  )
		`, pq.Array(groupIDs))
		if err != nil {
			return nil, err
		}
		result.GroupModelRoutingCleared = n
	}

	if len(accountIDs) > 0 || len(groupIDs) > 0 {
		n, err := execRowsAffected(ctx, tx, `
			DELETE FROM account_groups ag
			USING accounts a, groups g
			WHERE ag.account_id = a.id
			  AND ag.group_id = g.id
			  AND a.project_id <> g.project_id
			  AND (
				ag.account_id = ANY($1)
				OR ag.group_id = ANY($2)
			  )
		`, pq.Array(accountIDs), pq.Array(groupIDs))
		if err != nil {
			return nil, err
		}
		result.AccountGroupBindingsRemoved = n
	}

	if len(apiKeyIDs) > 0 || len(groupIDs) > 0 {
		n, err := execRowsAffected(ctx, tx, `
			UPDATE api_keys ak
			SET group_id = NULL,
				updated_at = CURRENT_TIMESTAMP
			FROM groups g
			WHERE ak.group_id = g.id
			  AND ak.deleted_at IS NULL
			  AND ak.project_id <> g.project_id
			  AND (
				ak.id = ANY($1)
				OR ak.group_id = ANY($2)
			  )
		`, pq.Array(apiKeyIDs), pq.Array(groupIDs))
		if err != nil {
			return nil, err
		}
		result.APIKeyGroupBindingsCleared = n
	}

	if len(result.InvalidatedUserIDs) > 0 {
		n, err := execRowsAffected(ctx, tx, `
			INSERT INTO project_members (project_id, user_id, role, scopes, is_owner, created_at, updated_at)
			SELECT $1, u.id, $3, '[]'::jsonb, FALSE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
			FROM users u
			WHERE u.id = ANY($2)
			  AND u.deleted_at IS NULL
			ON CONFLICT (project_id, user_id) DO NOTHING
		`, projectID, pq.Array(result.InvalidatedUserIDs), service.ProjectRoleUser)
		if err != nil {
			return nil, err
		}
		result.ProjectMembersAdded = n
	}

	if input.MoveUsageHistory {
		n, err := execRowsAffected(ctx, tx, `
			UPDATE usage_logs
			SET project_id = $4
			WHERE project_id <> $4
			  AND (
				account_id = ANY($1)
				OR api_key_id = ANY($2)
				OR group_id = ANY($3)
			  )
		`, pq.Array(accountIDs), pq.Array(apiKeyIDs), pq.Array(groupIDs), projectID)
		if err != nil {
			return nil, err
		}
		result.UsageLogsMoved = n

		n, err = execRowsAffected(ctx, tx, `
			UPDATE ops_error_logs
			SET project_id = $4
			WHERE project_id <> $4
			  AND (
				account_id = ANY($1)
				OR api_key_id = ANY($2)
				OR group_id = ANY($3)
			  )
		`, pq.Array(accountIDs), pq.Array(apiKeyIDs), pq.Array(groupIDs), projectID)
		if err != nil {
			return nil, err
		}
		result.OpsErrorLogsMoved = n
	}

	if err := enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventFullRebuild, nil, nil, map[string]any{
		"reason":     "project_resource_move",
		"project_id": projectID,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func loadProjectMoveInvalidationScope(ctx context.Context, tx *sql.Tx, accountIDs, apiKeyIDs, groupIDs []int64, result *service.ProjectResourceMoveResult) error {
	if result == nil {
		return nil
	}
	if len(apiKeyIDs) > 0 || len(groupIDs) > 0 {
		keys, err := queryStringColumn(ctx, tx, `
			SELECT DISTINCT key
			FROM api_keys
			WHERE deleted_at IS NULL
			  AND (
				id = ANY($1)
				OR group_id = ANY($2)
			  )
			ORDER BY key ASC
		`, pq.Array(apiKeyIDs), pq.Array(groupIDs))
		if err != nil {
			return err
		}
		result.InvalidatedAPIKeys = keys
	}
	if len(groupIDs) > 0 {
		result.InvalidatedGroupIDs = append([]int64(nil), groupIDs...)
		userIDs, err := queryInt64Column(ctx, tx, `
			SELECT DISTINCT user_id
			FROM (
				SELECT user_id FROM user_allowed_groups WHERE group_id = ANY($1)
				UNION
				SELECT user_id FROM user_subscriptions WHERE group_id = ANY($1) AND deleted_at IS NULL
				UNION
				SELECT user_id FROM api_keys WHERE group_id = ANY($1) AND deleted_at IS NULL
			) scoped_users
			ORDER BY user_id ASC
		`, pq.Array(groupIDs))
		if err != nil {
			return err
		}
		result.InvalidatedUserIDs = userIDs
	}
	if len(apiKeyIDs) > 0 {
		userIDs, err := queryInt64Column(ctx, tx, `
			SELECT DISTINCT user_id
			FROM api_keys
			WHERE id = ANY($1)
			  AND deleted_at IS NULL
			ORDER BY user_id ASC
		`, pq.Array(apiKeyIDs))
		if err != nil {
			return err
		}
		result.InvalidatedUserIDs = mergeSortedInt64(result.InvalidatedUserIDs, userIDs)
	}
	_ = accountIDs
	return nil
}

func execRowsAffected(ctx context.Context, exec sqlExecutor, query string, args ...any) (int64, error) {
	res, err := exec.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func queryStringColumn(ctx context.Context, q sqlQueryer, query string, args ...any) ([]string, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]string, 0)
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func queryInt64Column(ctx context.Context, q sqlQueryer, query string, args ...any) ([]int64, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]int64, 0)
	for rows.Next() {
		var value int64
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func mergeSortedInt64(a, b []int64) []int64 {
	if len(a) == 0 {
		return append([]int64(nil), b...)
	}
	if len(b) == 0 {
		return a
	}
	seen := make(map[int64]struct{}, len(a)+len(b))
	out := make([]int64, 0, len(a)+len(b))
	for _, id := range append(a, b...) {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return strings.TrimSpace(*value)
}
