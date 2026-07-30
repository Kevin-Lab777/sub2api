import { describe, expect, it } from 'vitest'
import { resolveNextAPINavigation } from '@/router/nextApiGuard'
import { resolveCompletedSetupRedirectPath } from '@/router/setupRedirect'

describe('Next API navigation guard', () => {
  it('allows public login for an unauthenticated administrator', () => {
    expect(resolveNextAPINavigation('/login', { requiresAuth: false }, {
      isAuthenticated: false,
      isAdmin: false
    })).toBeNull()
  })

  it('redirects an authenticated administrator from login to account management', () => {
    expect(resolveNextAPINavigation('/login', { requiresAuth: false }, {
      isAuthenticated: true,
      isAdmin: true
    })).toBe('/admin/accounts')
  })

  it('requires authentication for account-pool management', () => {
    expect(resolveNextAPINavigation('/admin/groups', { requiresAdmin: true }, {
      isAuthenticated: false,
      isAdmin: false
    })).toBe('/login')
  })

  it('rejects a non-admin token from every protected route', () => {
    expect(resolveNextAPINavigation('/admin/accounts', { requiresAdmin: true }, {
      isAuthenticated: true,
      isAdmin: false
    })).toBe('/login')
  })

  it('keeps account-pool management available to administrators', () => {
    expect(resolveNextAPINavigation('/admin/groups', { requiresAdmin: true }, {
      isAuthenticated: true,
      isAdmin: true
    })).toBeNull()
  })
})

describe('completed setup redirect', () => {
  it('opens account management for an authenticated administrator', () => {
    expect(resolveCompletedSetupRedirectPath(true, true)).toBe('/admin/accounts')
  })

  it('returns every other identity to administrator login', () => {
    expect(resolveCompletedSetupRedirectPath(false, false)).toBe('/login')
    expect(resolveCompletedSetupRedirectPath(true, false)).toBe('/login')
  })
})
