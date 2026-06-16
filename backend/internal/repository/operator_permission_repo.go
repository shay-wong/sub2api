package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type operatorPermissionRepository struct {
	sql sqlExecutor
}

func NewOperatorPermissionRepository(sqlDB *sql.DB) service.OperatorPermissionRepository {
	return &operatorPermissionRepository{sql: sqlDB}
}

func (r *operatorPermissionRepository) ListOperatorPermissionSubjects(ctx context.Context) ([]service.OperatorPermissionSubject, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT
			u.id,
			u.email,
			COALESCE(u.username, ''),
			u.role,
			u.status,
			COALESCE(array_remove(array_agg(ogs.group_id ORDER BY ogs.group_id), NULL), '{}')::bigint[],
			u.created_at,
			u.updated_at
		FROM users u
		LEFT JOIN operator_group_scopes ogs ON ogs.user_id = u.id
		WHERE u.deleted_at IS NULL
		  AND u.role IN ($1, $2)
		GROUP BY u.id, u.email, u.username, u.role, u.status, u.created_at, u.updated_at
		ORDER BY
			CASE WHEN u.role = $2 THEN 0 ELSE 1 END,
			u.id ASC
	`, service.RoleUser, service.RoleOperator)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.OperatorPermissionSubject, 0)
	for rows.Next() {
		var item service.OperatorPermissionSubject
		var groupIDs []int64
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&item.ID, &item.Email, &item.Username, &item.Role, &item.Status, pq.Array(&groupIDs), &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		item.GroupIDs = groupIDs
		item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *operatorPermissionRepository) GetOperatorGroupIDs(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT group_id
		FROM operator_group_scopes
		WHERE user_id = $1
		ORDER BY group_id ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *operatorPermissionRepository) GetOperatorScopesByUserIDs(ctx context.Context, userIDs []int64) (map[int64][]int64, error) {
	out := make(map[int64][]int64, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT user_id, group_id
		FROM operator_group_scopes
		WHERE user_id = ANY($1)
		ORDER BY user_id ASC, group_id ASC
	`, pq.Array(userIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var userID, groupID int64
		if err := rows.Scan(&userID, &groupID); err != nil {
			return nil, err
		}
		out[userID] = append(out[userID], groupID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *operatorPermissionRepository) SetOperatorGroupIDs(ctx context.Context, userID int64, groupIDs []int64, createdBy *int64) error {
	tx, err := beginSQLTx(ctx, r.sql)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, "DELETE FROM operator_group_scopes WHERE user_id = $1", userID); err != nil {
		return err
	}
	for _, groupID := range groupIDs {
		if groupID <= 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO operator_group_scopes (user_id, group_id, created_by)
			VALUES ($1, $2, $3)
			ON CONFLICT (user_id, group_id) DO NOTHING
		`, userID, groupID, createdBy); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *operatorPermissionRepository) ClearOperatorGroupIDs(ctx context.Context, userID int64) error {
	_, err := r.sql.ExecContext(ctx, "DELETE FROM operator_group_scopes WHERE user_id = $1", userID)
	return err
}

func beginSQLTx(ctx context.Context, exec sqlExecutor) (*sql.Tx, error) {
	if tx, ok := exec.(*sql.Tx); ok {
		return tx, nil
	}
	db, ok := exec.(*sql.DB)
	if !ok {
		return nil, sql.ErrConnDone
	}
	return db.BeginTx(ctx, nil)
}
