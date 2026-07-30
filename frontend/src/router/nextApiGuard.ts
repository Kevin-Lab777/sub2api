export interface NextAPIAuthState {
  isAuthenticated: boolean
  isAdmin: boolean
}

export function resolveNextAPINavigation(
  path: string,
  meta: Record<string, unknown>,
  auth: NextAPIAuthState
): string | null {
  const requiresAuth = meta.requiresAuth !== false

  if (!requiresAuth) {
    if (path === '/login' && auth.isAuthenticated && auth.isAdmin) {
      return '/admin/accounts'
    }
    return null
  }

  if (!auth.isAuthenticated || !auth.isAdmin) {
    return '/login'
  }

  return null
}
