package service

import "context"

const DefaultProjectSlug = "default"
const DefaultProjectName = "默认项目"

type projectContextKey struct{}
type requireActiveProjectMemberContextKey struct{}
type adminRoleContextKey struct{}
type adminPermissionsContextKey struct{}

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

func WithRequireActiveProjectMember(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, requireActiveProjectMemberContextKey{}, true)
}

func RequireActiveProjectMemberFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	required, ok := ctx.Value(requireActiveProjectMemberContextKey{}).(bool)
	return ok && required
}

func WithAdminRole(ctx context.Context, role string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if role == "" {
		return ctx
	}
	return context.WithValue(ctx, adminRoleContextKey{}, role)
}

func AdminRoleFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	role, ok := ctx.Value(adminRoleContextKey{}).(string)
	return role, ok && role != ""
}

func WithAdminPermissions(ctx context.Context, permissions []string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, adminPermissionsContextKey{}, append([]string(nil), permissions...))
}

func AdminPermissionsFromContext(ctx context.Context) ([]string, bool) {
	if ctx == nil {
		return nil, false
	}
	permissions, ok := ctx.Value(adminPermissionsContextKey{}).([]string)
	if !ok {
		return nil, false
	}
	return append([]string(nil), permissions...), true
}

type withoutProjectIDContext struct {
	context.Context
}

func (c withoutProjectIDContext) Value(key any) any {
	if _, ok := key.(projectContextKey); ok {
		return nil
	}
	return c.Context.Value(key)
}

// WithoutProjectID returns a context that keeps cancellation/deadline metadata while
// removing the project scope value for internal maintenance work.
func WithoutProjectID(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return withoutProjectIDContext{Context: ctx}
}
