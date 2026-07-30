import { describe, expect, it } from 'vitest'
import { routes } from '@/router'

function flattenPaths(): string[] {
  return routes.flatMap((route) => [route.path, ...(route.alias ? [String(route.alias)] : [])])
}

describe('Next API route surface', () => {
  const paths = flattenPaths()

  it('keeps technical account-pool and account management', () => {
    expect(paths).toContain('/admin/groups')
    expect(paths).toContain('/admin/accounts')
    expect(paths).toContain('/admin/proxies')
  })

  it.each([
    '/register',
    '/dashboard',
    '/keys',
    '/subscriptions',
    '/redeem',
    '/purchase',
    '/payment/result',
    '/model-plaza',
    '/admin/users',
    '/admin/subscriptions',
    '/admin/redeem',
    '/admin/promo-codes',
    '/admin/orders'
  ])('does not register customer route %s', (path) => {
    expect(paths).not.toContain(path)
  })
})
