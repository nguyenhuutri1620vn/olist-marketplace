import type { Role } from '@/lib/routes'

export type Session = { token: string; email: string; accountId: string; role: Role; exp: number }

type Claims = { sub: string; role: Role; exp: number }

const key = 'olist.session'
const roles: Role[] = ['customer', 'seller', 'admin']

// decodeClaims reads sub, role and exp from a JWT without verifying it; the
// gateway and services verify (AD-14). It returns null for anything unusable.
export function decodeClaims(token: string): Claims | null {
  const part = token.split('.')[1]
  if (!part) return null
  try {
    const json = atob(part.replace(/-/g, '+').replace(/_/g, '/'))
    const c = JSON.parse(json) as Partial<Claims>
    if (typeof c.sub !== 'string' || !roles.includes(c.role as Role) || typeof c.exp !== 'number') return null
    return { sub: c.sub, role: c.role as Role, exp: c.exp }
  } catch {
    return null
  }
}

// saveSession keeps the token and the email typed at login (the JWT carries
// no email) for this browser tab.
export function saveSession(token: string, email: string): void {
  sessionStorage.setItem(key, JSON.stringify({ token, email }))
}

// loadSession returns the stored session if its token is still unexpired at
// nowSeconds.
export function loadSession(nowSeconds = Date.now() / 1000): Session | null {
  try {
    const raw = sessionStorage.getItem(key)
    if (!raw) return null
    const { token, email } = JSON.parse(raw) as { token: string; email: string }
    const c = decodeClaims(token)
    if (!c || c.exp <= nowSeconds) return null
    return { token, email, accountId: c.sub, role: c.role, exp: c.exp }
  } catch {
    return null
  }
}

// storedSessionExpired reports whether a session was stored but its token has
// expired, so the app can say "Session expired" instead of a plain login.
export function storedSessionExpired(nowSeconds = Date.now() / 1000): boolean {
  try {
    const raw = sessionStorage.getItem(key)
    if (!raw) return false
    const c = decodeClaims((JSON.parse(raw) as { token: string }).token)
    return c !== null && c.exp <= nowSeconds
  } catch {
    return false
  }
}

export function clearSession(): void {
  sessionStorage.removeItem(key)
}
