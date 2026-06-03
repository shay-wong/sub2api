package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type userGroupRateLimitWindowRepository struct {
	sql sqlExecutor
}

func NewUserGroupRateLimitWindowRepository(sqlDB *sql.DB) service.UserGroupRateLimitWindowRepository {
	return &userGroupRateLimitWindowRepository{sql: sqlDB}
}

func (r *userGroupRateLimitWindowRepository) Get(ctx context.Context, userID, groupID int64) (*service.UserGroupRateLimitWindowRecord, error) {
	row, err := scanUserGroupRateLimitWindow(ctx, r.sql, `
		SELECT w.user_id, w.group_id, g.name, g.rate_limit_5h, w.usage_5h_usd, w.window_5h_start
		FROM user_group_rate_limit_windows w
		JOIN users u ON u.id = w.user_id AND u.deleted_at IS NULL
		JOIN groups g ON g.id = w.group_id AND g.deleted_at IS NULL
		WHERE w.user_id = $1
			AND w.group_id = $2
			AND w.deleted_at IS NULL
	`, userID, groupID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return row, err
}

func (r *userGroupRateLimitWindowRepository) ListByUser(ctx context.Context, userID int64) ([]service.UserGroupRateLimitWindowRecord, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT
			$1::bigint AS user_id,
			g.id AS group_id,
			g.name,
			g.rate_limit_5h,
			COALESCE(w.usage_5h_usd, 0) AS usage_5h_usd,
			w.window_5h_start
		FROM groups g
		JOIN users u ON u.id = $1 AND u.deleted_at IS NULL
		LEFT JOIN user_group_rate_limit_windows w
			ON w.group_id = g.id
			AND w.user_id = u.id
			AND w.deleted_at IS NULL
		WHERE g.deleted_at IS NULL
		ORDER BY g.name, g.id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []service.UserGroupRateLimitWindowRecord
	for rows.Next() {
		rec, err := scanUserGroupRateLimitWindowRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *userGroupRateLimitWindowRepository) ListByGroup(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]service.UserGroupRateLimitWindowRecord, *pagination.PaginationResult, error) {
	var total int64
	totalRows, err := r.sql.QueryContext(ctx, `
		SELECT COUNT(*)
		FROM user_group_rate_limit_windows w
		JOIN users u ON u.id = w.user_id AND u.deleted_at IS NULL
		JOIN groups g ON g.id = w.group_id AND g.deleted_at IS NULL
		WHERE w.group_id = $1
			AND w.deleted_at IS NULL
	`, groupID)
	if err != nil {
		return nil, nil, err
	}
	if totalRows.Next() {
		if err := totalRows.Scan(&total); err != nil {
			_ = totalRows.Close()
			return nil, nil, err
		}
	}
	if err := totalRows.Err(); err != nil {
		_ = totalRows.Close()
		return nil, nil, err
	}
	if err := totalRows.Close(); err != nil {
		return nil, nil, err
	}

	rows, err := r.sql.QueryContext(ctx, `
		SELECT w.user_id, w.group_id, g.name, g.rate_limit_5h, w.usage_5h_usd, w.window_5h_start
		FROM user_group_rate_limit_windows w
		JOIN users u ON u.id = w.user_id AND u.deleted_at IS NULL
		JOIN groups g ON g.id = w.group_id AND g.deleted_at IS NULL
		WHERE w.group_id = $1
			AND w.deleted_at IS NULL
		ORDER BY w.usage_5h_usd DESC, w.user_id ASC
		LIMIT $2 OFFSET $3
	`, groupID, params.Limit(), params.Offset())
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.UserGroupRateLimitWindowRecord, 0, params.Limit())
	for rows.Next() {
		rec, err := scanUserGroupRateLimitWindowRows(rows)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, *rec)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return out, paginationResultFromTotal(total, params), nil
}

func (r *userGroupRateLimitWindowRepository) IncrementWithWindowReset(ctx context.Context, userID, groupID int64, cost float64, now time.Time) error {
	if cost <= 0 {
		return nil
	}
	res, err := r.sql.ExecContext(ctx, `
		INSERT INTO user_group_rate_limit_windows (
			user_id,
			group_id,
			usage_5h_usd,
			window_5h_start,
			created_at,
			updated_at
		)
		SELECT u.id, g.id, $3, $4, $4, $4
		FROM users u
		JOIN groups g ON g.id = $2 AND g.deleted_at IS NULL
		WHERE u.id = $1 AND u.deleted_at IS NULL
		ON CONFLICT (user_id, group_id) WHERE deleted_at IS NULL
		DO UPDATE SET
			usage_5h_usd = CASE
				WHEN user_group_rate_limit_windows.window_5h_start IS NULL
					OR user_group_rate_limit_windows.window_5h_start + INTERVAL '5 hours' <= $4
				THEN EXCLUDED.usage_5h_usd
				ELSE user_group_rate_limit_windows.usage_5h_usd + EXCLUDED.usage_5h_usd
			END,
			window_5h_start = CASE
				WHEN user_group_rate_limit_windows.window_5h_start IS NULL
					OR user_group_rate_limit_windows.window_5h_start + INTERVAL '5 hours' <= $4
				THEN EXCLUDED.window_5h_start
				ELSE user_group_rate_limit_windows.window_5h_start
			END,
			updated_at = EXCLUDED.updated_at
	`, userID, groupID, cost, now)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrGroupNotFound
	}
	return nil
}

func (r *userGroupRateLimitWindowRepository) Reset(ctx context.Context, userID, groupID int64) (*service.UserGroupRateLimitWindowRecord, error) {
	rec, err := scanUserGroupRateLimitWindow(ctx, r.sql, `
		INSERT INTO user_group_rate_limit_windows (
			user_id,
			group_id,
			usage_5h_usd,
			window_5h_start,
			created_at,
			updated_at
		)
		SELECT u.id, g.id, 0, NULL, NOW(), NOW()
		FROM users u
		JOIN groups g ON g.id = $2 AND g.deleted_at IS NULL
		WHERE u.id = $1 AND u.deleted_at IS NULL
		ON CONFLICT (user_id, group_id) WHERE deleted_at IS NULL
		DO UPDATE SET
			usage_5h_usd = 0,
			window_5h_start = NULL,
			updated_at = NOW()
		RETURNING
			user_group_rate_limit_windows.user_id,
			user_group_rate_limit_windows.group_id,
			(SELECT name FROM groups WHERE id = user_group_rate_limit_windows.group_id AND deleted_at IS NULL),
			(SELECT rate_limit_5h FROM groups WHERE id = user_group_rate_limit_windows.group_id AND deleted_at IS NULL),
			user_group_rate_limit_windows.usage_5h_usd,
			user_group_rate_limit_windows.window_5h_start
	`, userID, groupID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrGroupNotFound
	}
	return rec, err
}

type userGroupRateLimitWindowScanner interface {
	Scan(dest ...any) error
}

func scanUserGroupRateLimitWindow(ctx context.Context, q sqlExecutor, query string, args ...any) (*service.UserGroupRateLimitWindowRecord, error) {
	row, ok := q.(interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	})
	if ok {
		return scanUserGroupRateLimitWindowRow(row.QueryRowContext(ctx, query, args...))
	}
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, sql.ErrNoRows
	}
	rec, err := scanUserGroupRateLimitWindowRows(rows)
	if err != nil {
		return nil, err
	}
	return rec, rows.Err()
}

func scanUserGroupRateLimitWindowRow(row userGroupRateLimitWindowScanner) (*service.UserGroupRateLimitWindowRecord, error) {
	var rec service.UserGroupRateLimitWindowRecord
	var groupName sql.NullString
	var rateLimit sql.NullFloat64
	var windowStart sql.NullTime
	if err := row.Scan(&rec.UserID, &rec.GroupID, &groupName, &rateLimit, &rec.Usage5hUSD, &windowStart); err != nil {
		return nil, err
	}
	if groupName.Valid {
		rec.GroupName = groupName.String
	}
	if rateLimit.Valid {
		rec.RateLimit5h = rateLimit.Float64
	}
	if windowStart.Valid {
		rec.Window5hStart = &windowStart.Time
	}
	return &rec, nil
}

func scanUserGroupRateLimitWindowRows(rows *sql.Rows) (*service.UserGroupRateLimitWindowRecord, error) {
	return scanUserGroupRateLimitWindowRow(rows)
}
