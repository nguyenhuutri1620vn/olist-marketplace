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

export type FakeProduct = {
  product_id: string
  display_name: string
  category_name_pt: string
  category_name_en: string
  seller_id: string
  seller_city: string
  seller_state: string
  price_centavos: number
}

export const categories = [
  { name_pt: 'beleza_saude', name_en: 'health_beauty' },
  { name_pt: 'pc_gamer', name_en: 'pc_gamer' },
  { name_pt: 'Uncategorized', name_en: 'Uncategorized' },
]

function fakeProduct(i: number, cat: (typeof categories)[number]): FakeProduct {
  const id = i.toString(16).padStart(4, '0') + 'f'.repeat(28)
  return {
    product_id: id,
    display_name: `${cat.name_en} #${id.slice(0, 4)}`,
    category_name_pt: cat.name_pt,
    category_name_en: cat.name_en,
    seller_id: '6560211a19b47992c3666cc44a7e94c0',
    seller_city: 'ibitinga',
    seller_state: 'SP',
    price_centavos: 100000 + i,
  }
}

// catalog has 45 health_beauty Products, no pc_gamer ones and 3
// Uncategorized ones, in the gateway's order.
export function fakeCatalog(): FakeProduct[] {
  const out: FakeProduct[] = []
  for (let i = 0; i < 45; i++) out.push(fakeProduct(i, categories[0]))
  for (let i = 45; i < 48; i++) out.push(fakeProduct(i, categories[2]))
  return out
}

// fakeGateway answers the auth and browsing routes like api-gateway and
// records every request. Set products to change the catalog, or fail to the
// browsing route paths that should answer 503.
export function fakeGateway() {
  const calls: { url: string; init?: RequestInit }[] = []
  const registered = new Set<string>()
  const state = { products: fakeCatalog(), fail: [] as string[] }
  const fetch = async (url: string, init?: RequestInit): Promise<Response> => {
    calls.push({ url, init })
    const u = new URL(url, 'http://web')
    if (u.pathname === '/api/categories' || u.pathname === '/api/products') {
      if (state.fail.includes(u.pathname)) return json(503, { error: { code: 'UNAVAILABLE', message: 'Service Unavailable' } })
      if (u.pathname === '/api/categories') return json(200, { categories })
      const category = u.searchParams.get('category') ?? ''
      const page = Number(u.searchParams.get('page') ?? '1')
      const size = Number(u.searchParams.get('page_size') ?? '20')
      // The same bounds as the gateway and catalog-service.
      if (!Number.isInteger(page) || page < 1 || page > 2 ** 31 - 1 || !Number.isInteger(size) || size < 1 || size > 100) {
        return json(400, { error: { code: 'INVALID_ARGUMENT', message: 'bad page' } })
      }
      if (category && !categories.some((c) => c.name_pt === category)) {
        return json(400, { error: { code: 'INVALID_ARGUMENT', message: `invalid: unknown category "${category}"` } })
      }
      const all = state.products.filter((p) => !category || p.category_name_pt === category)
      return json(200, { products: all.slice((page - 1) * size, page * size), total: all.length, page, page_size: size })
    }
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
  return { fetch, calls, registered, state }
}
