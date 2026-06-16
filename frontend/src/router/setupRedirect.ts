export function resolveCompletedSetupRedirectPath(isAuthenticated: boolean, canAccessAdminConsole: boolean): string {
  if (!isAuthenticated) {
    return '/login'
  }

  return canAccessAdminConsole ? '/admin/dashboard' : '/dashboard'
}
