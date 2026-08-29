package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const (
	defaultProjectSlug = "default"
	defaultProjectName = "默认项目"
)

// Legacy project_id columns remain NOT NULL until the contract migration drops
// them. New records are assigned to the single default row during this window.
func resolveProjectID(ctx context.Context, sqlq sqlExecutor) (int64, error) {
	return ensureDefaultProject(ctx, sqlq)
}

func resolveProjectIDForCreate(ctx context.Context, sqlq sqlExecutor, _ int64) (int64, error) {
	return ensureDefaultProject(ctx, sqlq)
}

func ensureDefaultProject(ctx context.Context, sqlq sqlExecutor) (int64, error) {
	if sqlq == nil {
		return 0, fmt.Errorf("default project lookup requires sql executor")
	}
	var projectID int64
	err := scanSingleRow(ctx, sqlq, `SELECT id FROM projects WHERE slug = $1`, []any{defaultProjectSlug}, &projectID)
	if err == nil {
		return projectID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	_, err = sqlq.ExecContext(ctx, `
			INSERT INTO projects (name, slug, description, status, profiles, created_at, updated_at)
			VALUES ($1, $2, $3, 'active', $4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			ON CONFLICT (slug) DO NOTHING
		`, defaultProjectName, defaultProjectSlug, "Legacy default project for required project_id columns.", "{}")
	if err != nil {
		return 0, err
	}
	err = scanSingleRow(ctx, sqlq, `SELECT id FROM projects WHERE slug = $1`, []any{defaultProjectSlug}, &projectID)
	return projectID, err
}
