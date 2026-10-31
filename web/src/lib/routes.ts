export type Role = 'customer' | 'seller' | 'admin'

const homes: Record<Role, string> = { customer: '/', seller: '/seller/orders', admin: '/admin' }

// homeFor is where a role lands after login (EXPERIENCE.md role routing).
export function homeFor(role: Role): string {
  return homes[role]
}

const owners: [prefix: string, role: Role][] = [
  ['/checkout', 'customer'],
  ['/orders', 'customer'],
  ['/seller', 'seller'],
  ['/admin', 'admin'],
]

// roleOfPath returns the role a route belongs to, or null for public routes.
export function roleOfPath(path: string): Role | null {
  // The router matches case-insensitively, so ownership must too.
  const p = path.toLowerCase()
  for (const [prefix, role] of owners) {
    if (p === prefix || p.startsWith(prefix + '/')) return role
  }
  return null
}

// canOpen reports whether role may open path; public routes are open to all.
export function canOpen(path: string, role: Role | null): boolean {
  const owner = roleOfPath(path)
  return owner === null || owner === role
}

// safeReturn turns the post-login return-to value into a route for role: only
// a same-origin path starting with a single "/" that role may open, and not
// the login or register page; anything else goes to the role's home (UX-DR6).
export function safeReturn(next: string | null, role: Role): string {
  if (!next || !next.startsWith('/') || next.startsWith('//') || next.startsWith('/\\')) return homeFor(role)
  const url = new URL(next, 'http://same.origin')
  if (url.origin !== 'http://same.origin') return homeFor(role)
  const page = url.pathname.toLowerCase().replace(/\/+$/, '')
  if (page === '/login' || page === '/register') return homeFor(role)
  if (!canOpen(url.pathname, role)) return homeFor(role)
  return url.pathname + url.search + url.hash
}

// loginURL is the Login route that returns to path after login; expired adds
// the "Session expired" message.
export function loginURL(path: string, expired: boolean): string {
  const q = new URLSearchParams({ next: path })
  if (expired) q.set('expired', '1')
  return `/login?${q}`
}
