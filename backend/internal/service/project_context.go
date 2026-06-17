package service

import "context"

const DefaultProjectSlug = "default"
const DefaultProjectName = "默认项目"

type projectContextKey struct{}

func WithProjectID(ctx context.Context, projectID int64) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if projectID <= 0 {
		return ctx
	}
	return context.WithValue(ctx, projectContextKey{}, projectID)
}

func ProjectIDFromContext(ctx context.Context) (int64, bool) {
	if ctx == nil {
		return 0, false
	}
	id, ok := ctx.Value(projectContextKey{}).(int64)
	return id, ok && id > 0
}
