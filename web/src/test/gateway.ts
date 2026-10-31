import { fakeJWT } from '@/test/jwt'

// demo maps demo emails to the role their token carries.
export const demo = {
  customer: 'customer-8d50f5ea@demo.olist.local',
  seller: 'seller-6560211a@demo.olist.local',
  admin: 'admin@demo.olist.local',
} as const

export const demoPassword = 'olist-demo-2026'

export function tokenFor(role: string, expInSeconds = 3600): string {
  return fakeJWT({ sub: `${role}-id`, role, exp: Math.floor(Date.now() / 1000) + expInSeconds })
}

function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

// fakeGateway answers /api/auth/login and /api/auth/register like api-gateway
// and records every request.
export function fakeGateway() {
  const calls: { url: string; init?: RequestInit }[] = []
  const registered = new Set<string>()
  const fetch = async (url: string, init?: RequestInit): Promise<Response> => {
    calls.push({ url, init })
    const body = init?.body ? JSON.parse(String(init.body)) : {}
    if (url === '/api/auth/login') {
      for (const [role, email] of Object.entries(demo)) {
        if (body.email === email && body.password === demoPassword) {
          return json(200, { access_token: tokenFor(role), token_type: 'Bearer', expires_in: 3600 })
        }
      }
      if (registered.has(body.email)) return json(200, { access_token: tokenFor('customer'), token_type: 'Bearer', expires_in: 3600 })
      return json(401, { error: { code: 'UNAUTHENTICATED', message: 'invalid email or password' } })
    }
    if (url === '/api/auth/register') {
      if (registered.has(body.email)) return json(409, { error: { code: 'ALREADY_EXISTS', message: 'email already registered' } })
      registered.add(body.email)
      return json(200, { access_token: tokenFor('customer'), token_type: 'Bearer', expires_in: 3600 })
    }
    if (url === '/api/expired-probe') return json(401, { error: { code: 'UNAUTHENTICATED', message: 'invalid or expired token' } })
    return json(404, { error: { code: 'NOT_FOUND', message: 'no route' } })
  }
  return { fetch, calls, registered }
}
