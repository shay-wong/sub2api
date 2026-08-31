package repository

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

func (r *operatorPermissionRepository) GetAdminResourceScope(ctx context.Context, userID int64) (service.AdminResourceScope, error) {
	scope := service.UnrestrictedAdminResourceScope()
	rows, err := r.sql.QueryContext(ctx, `
		SELECT mode
		FROM admin_resource_scopes
		WHERE user_id = $1
	`, userID)
	if err != nil {
		return service.AdminResourceScope{}, err
	}
	if !rows.Next() {
		_ = rows.Close()
		return scope, nil
	}
	if err := rows.Scan(&scope.Mode); err != nil {
		_ = rows.Close()
		return service.AdminResourceScope{}, err
	}
	if err := rows.Close(); err != nil {
		return service.AdminResourceScope{}, err
	}
	rows, err = r.sql.QueryContext(ctx, `
		SELECT resource_type, resource_id
		FROM admin_resource_bindings
		WHERE user_id = $1
		ORDER BY resource_type ASC, resource_id ASC
	`, userID)
	if err != nil {
		return service.AdminResourceScope{}, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var resourceType string
		var resourceID int64
		if err := rows.Scan(&resourceType, &resourceID); err != nil {
			return service.AdminResourceScope{}, err
		}
		appendAdminResourceID(&scope, resourceType, resourceID)
	}
	if err := rows.Err(); err != nil {
		return service.AdminResourceScope{}, err
	}
	return scope, nil
}

func (r *operatorPermissionRepository) GetAdminResourceScopesByUserIDs(ctx context.Context, userIDs []int64) (map[int64]service.AdminResourceScope, error) {
	out := make(map[int64]service.AdminResourceScope, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT ars.user_id, ars.mode, arb.resource_type, arb.resource_id
		FROM admin_resource_scopes ars
		LEFT JOIN admin_resource_bindings arb ON arb.user_id = ars.user_id
		WHERE ars.user_id = ANY($1)
		ORDER BY ars.user_id ASC, arb.resource_type ASC, arb.resource_id ASC
	`, pq.Array(userIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var userID int64
		var mode string
		var resourceType sql.NullString
		var resourceID sql.NullInt64
		if err := rows.Scan(&userID, &mode, &resourceType, &resourceID); err != nil {
			return nil, err
		}
		scope, ok := out[userID]
		if !ok {
			scope = service.UnrestrictedAdminResourceScope()
			scope.Mode = mode
		}
		if resourceType.Valid && resourceID.Valid {
			appendAdminResourceID(&scope, resourceType.String, resourceID.Int64)
		}
		out[userID] = scope
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *operatorPermissionRepository) UpdateUserAdminAccess(ctx context.Context, userID int64, role string, permissions []string, scope service.AdminResourceScope, createdBy *int64) error {
	tx, err := beginSQLTx(ctx, r.sql)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	permissionsJSON, err := json.Marshal(permissions)
	if err != nil {
		return err
	}
	if role == service.RoleAdmin && scope.Mode == service.AdminResourceScopeRestricted {
		var groupCount, accountCount, proxyCount, subscriptionCount int
		if err := tx.QueryRowContext(ctx, `
			SELECT
				(SELECT COUNT(*) FROM groups WHERE id = ANY($1) AND deleted_at IS NULL),
				(SELECT COUNT(*) FROM accounts WHERE id = ANY($2) AND deleted_at IS NULL),
				(SELECT COUNT(*) FROM proxies WHERE id = ANY($3) AND deleted_at IS NULL),
				(SELECT COUNT(*) FROM user_subscriptions WHERE id = ANY($4) AND deleted_at IS NULL)
		`, pq.Array(scope.GroupIDs), pq.Array(scope.AccountIDs), pq.Array(scope.ProxyIDs), pq.Array(scope.SubscriptionIDs)).Scan(
			&groupCount, &accountCount, &proxyCount, &subscriptionCount,
		); err != nil {
			return err
		}
		if groupCount != len(scope.GroupIDs) || accountCount != len(scope.AccountIDs) || proxyCount != len(scope.ProxyIDs) || subscriptionCount != len(scope.SubscriptionIDs) {
			return service.ErrAdminResourceScopeInvalid
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE users
		SET role = $2, admin_permissions = $3::jsonb, updated_at = NOW()
		WHERE id = $1
	`, userID, role, string(permissionsJSON)); err != nil {
		return err
	}
	if role != service.RoleAdmin {
		if _, err := tx.ExecContext(ctx, "DELETE FROM admin_resource_scopes WHERE user_id = $1", userID); err != nil {
			return err
		}
		return tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO admin_resource_scopes (user_id, mode, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (user_id) DO UPDATE SET mode = EXCLUDED.mode, updated_at = NOW()
	`, userID, scope.Mode, createdBy); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM admin_resource_bindings WHERE user_id = $1", userID); err != nil {
		return err
	}
	if scope.Mode == service.AdminResourceScopeRestricted {
		for resourceType, resourceIDs := range adminResourceIDs(scope) {
			for _, resourceID := range resourceIDs {
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO admin_resource_bindings (user_id, resource_type, resource_id, created_by, created_at)
					VALUES ($1, $2, $3, $4, NOW())
					ON CONFLICT (user_id, resource_type, resource_id) DO NOTHING
				`, userID, resourceType, resourceID, createdBy); err != nil {
					return err
				}
			}
		}
	}
	return tx.Commit()
}

func (r *operatorPermissionRepository) BindAdminResource(ctx context.Context, userID int64, resourceType string, resourceID int64, createdBy *int64) error {
	_, err := r.sql.ExecContext(ctx, `
		INSERT INTO admin_resource_bindings (user_id, resource_type, resource_id, created_by, created_at)
		SELECT user_id, $2, $3, $4, NOW()
		FROM admin_resource_scopes
		WHERE user_id = $1 AND mode = 'restricted'
		ON CONFLICT (user_id, resource_type, resource_id) DO NOTHING
	`, userID, resourceType, resourceID, createdBy)
	return err
}

func appendAdminResourceID(scope *service.AdminResourceScope, resourceType string, resourceID int64) {
	switch resourceType {
	case service.AdminResourceGroup:
		scope.GroupIDs = append(scope.GroupIDs, resourceID)
	case service.AdminResourceAccount:
		scope.AccountIDs = append(scope.AccountIDs, resourceID)
	case service.AdminResourceProxy:
		scope.ProxyIDs = append(scope.ProxyIDs, resourceID)
	case service.AdminResourceSubscription:
		scope.SubscriptionIDs = append(scope.SubscriptionIDs, resourceID)
	}
}

func adminResourceIDs(scope service.AdminResourceScope) map[string][]int64 {
	return map[string][]int64{
		service.AdminResourceGroup:        scope.GroupIDs,
		service.AdminResourceAccount:      scope.AccountIDs,
		service.AdminResourceProxy:        scope.ProxyIDs,
		service.AdminResourceSubscription: scope.SubscriptionIDs,
	}
}
