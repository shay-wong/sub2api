package service

import "context"

type adminRoleContextKey struct{}
type adminPermissionsContextKey struct{}

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
