import { homeFor, safeReturn, roleOfPath, canOpen } from '@/lib/routes'

describe('homeFor', () => {
  it('sends each role to its home', () => {
    expect(homeFor('customer')).toBe('/')
    expect(homeFor('seller')).toBe('/seller/orders')
    expect(homeFor('admin')).toBe('/admin')
  })
})

describe('roleOfPath', () => {
  it.each([
    ['/', null],
    ['/login', null],
    ['/checkout', 'customer'],
    ['/orders', 'customer'],
    ['/orders/42', 'customer'],
    ['/seller/orders', 'seller'],
    ['/seller', 'seller'],
    ['/admin', 'admin'],
    ['/admin/vouchers', 'admin'],
    ['/sellerish', null],
    ['/administrator', null],
  ])('%s → %s', (path, role) => {
    expect(roleOfPath(path)).toBe(role)
  })
})

describe('canOpen', () => {
  it('lets anyone open public routes and only the owner open role routes', () => {
    expect(canOpen('/', 'seller')).toBe(true)
    expect(canOpen('/admin', 'admin')).toBe(true)
    expect(canOpen('/admin', 'seller')).toBe(false)
    expect(canOpen('/orders', null)).toBe(false)
  })
})

describe('safeReturn', () => {
  it.each([
    ['/orders/42?tab=items', 'customer', '/orders/42?tab=items'],
    ['/seller/revenue', 'seller', '/seller/revenue'],
    ['/', 'admin', '/'],
    [null, 'seller', '/seller/orders'],
    ['', 'admin', '/admin'],
    ['//evil.com', 'customer', '/'],
    ['/\\evil.com', 'customer', '/'],
    ['https://evil.com/admin', 'admin', '/admin'],
    ['javascript:alert(1)', 'admin', '/admin'],
    ['admin', 'admin', '/admin'],
    ['/admin', 'seller', '/seller/orders'],
    ['/seller/orders', 'customer', '/'],
    ['/login', 'customer', '/'],
    ['/register', 'admin', '/admin'],
    ['/ADMIN', 'seller', '/seller/orders'],
    ['/Login', 'customer', '/'],
    ['/login/', 'admin', '/admin'],
  ])('%s as %s → %s', (next, role, want) => {
    expect(safeReturn(next, role as 'customer' | 'seller' | 'admin')).toBe(want)
  })
})
