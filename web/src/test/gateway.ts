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
  freight_centavos: number
}

export type FakeOrder = {
  owner: string
  order_id: string
  purchased_at: string
  status: string
  seller_id: string
  voucher_amount_centavos: number
  items: { product_id: string; category_name_en: string; price_centavos: number; freight_centavos: number }[]
  timeline: { status: string; at: string }[]
  cancel_reason: string
  voucher_restored: boolean
}

// fakeOrders are 25 Orders of the demo Customer (account "customer-id"),
// newest first, one hour apart.
export function fakeOrders(): FakeOrder[] {
  const out: FakeOrder[] = []
  for (let i = 0; i < 25; i++) {
    const at = new Date(Date.UTC(2026, 10, 1, 12 - i, 0, 0)).toISOString().replace('.000', '')
    out.push({
      owner: 'customer-id',
      order_id: `0192f000-0000-7000-8000-${String(100 - i).padStart(12, '0')}`,
      purchased_at: at,
      status: 'created',
      seller_id: '6560211a19b47992c3666cc44a7e94c0',
      voucher_amount_centavos: 0,
      items: [
        { product_id: 'aaaa1111' + 'f'.repeat(24), category_name_en: 'health_beauty', price_centavos: 5890, freight_centavos: 1329 },
        { product_id: 'bbbb2222' + 'f'.repeat(24), category_name_en: 'Uncategorized', price_centavos: 760, freight_centavos: 0 },
      ],
      timeline: [{ status: 'created', at }],
      cancel_reason: '',
      voucher_restored: false,
    })
  }
  return out
}

function total(o: FakeOrder): number {
  return o.items.reduce((s, it) => s + it.price_centavos + it.freight_centavos, 0)
}

// callerOf reads the account id from a fake Bearer JWT.
function callerOf(init?: RequestInit): string | null {
  const auth = new Headers(init?.headers).get('Authorization')
  if (!auth) return null
  const payload = auth.replace(/^Bearer /, '').split('.')[1]
  return JSON.parse(atob(payload.replace(/-/g, '+').replace(/_/g, '/'))).sub
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
    // Uncategorized Products come from a second Seller.
    seller_id: cat.name_pt === 'Uncategorized' ? '7c67e1448b00f6e969d365cea6b010ab' : '6560211a19b47992c3666cc44a7e94c0',
    seller_city: cat.name_pt === 'Uncategorized' ? 'sao paulo' : 'ibitinga',
    seller_state: 'SP',
    price_centavos: 100000 + i,
    freight_centavos: 1000,
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
  const state = { products: fakeCatalog(), orders: fakeOrders(), fail: [] as string[], checkouts: new Map<string, unknown>(), checkoutDelayMs: 0, nextOrder: 0 }
  const fetch = async (url: string, init?: RequestInit): Promise<Response> => {
    calls.push({ url, init })
    const u = new URL(url, 'http://web')
    if (u.pathname === '/api/checkout' && init?.method === 'POST') {
      if (state.checkoutDelayMs) await new Promise((r) => setTimeout(r, state.checkoutDelayMs))
      if (state.fail.includes('/api/checkout#401')) return json(401, { error: { code: 'UNAUTHENTICATED', message: 'invalid or expired token' } })
      if (state.fail.includes('/api/checkout')) return json(503, { error: { code: 'UNAVAILABLE', message: 'catalog unavailable; nothing was created' } })
      const caller = callerOf(init)
      if (!caller) return json(401, { error: { code: 'UNAUTHENTICATED', message: 'missing bearer token' } })
      const { product_ids: ids, checkout_idempotency_key: key } = JSON.parse(String(init.body)) as { product_ids: string[]; checkout_idempotency_key: string }
      const seen = state.checkouts.get(`${caller}/${key}`)
      if (seen) return json(200, seen)
      const bySeller = new Map<string, FakeProduct[]>()
      for (const id of ids) {
        const p = state.products.find((x) => x.product_id === id)
        if (!p) return json(400, { error: { code: 'INVALID_ARGUMENT', message: `invalid: unknown product ids: ${id}` } })
        bySeller.set(p.seller_id, [...(bySeller.get(p.seller_id) ?? []), p])
      }
      const now = new Date().toISOString()
      const orders = [...bySeller.entries()].sort(([a], [b]) => a.localeCompare(b)).map(([seller, items]) => {
        const order_id = `0192f001-0000-7000-8000-${String(state.nextOrder++).padStart(12, '0')}`
        const fakeItems = items.map((p) => ({ product_id: p.product_id, category_name_en: p.category_name_en, price_centavos: p.price_centavos, freight_centavos: p.freight_centavos }))
        state.orders.unshift({ owner: caller, order_id, purchased_at: now, status: 'created', seller_id: seller, voucher_amount_centavos: 0, items: fakeItems, timeline: [{ status: 'created', at: now }], cancel_reason: '', voucher_restored: false })
        return { order_id, seller_id: seller, status: 'created', total_centavos: fakeItems.reduce((t, x) => t + x.price_centavos + x.freight_centavos, 0), items: fakeItems }
      })
      const resp = { checkout_id: `checkout-${state.checkouts.size + 1}`, orders }
      state.checkouts.set(`${caller}/${key}`, resp)
      return json(200, resp)
    }
    if (u.pathname === '/api/orders' || u.pathname.startsWith('/api/orders/')) {
      if (state.fail.includes('/api/orders')) return json(503, { error: { code: 'UNAVAILABLE', message: 'Service Unavailable' } })
      const caller = callerOf(init)
      if (!caller) return json(401, { error: { code: 'UNAUTHENTICATED', message: 'missing bearer token' } })
      const mine = state.orders.filter((o) => o.owner === caller)
      if (u.pathname === '/api/orders') {
        const page = Number(u.searchParams.get('page') ?? '1')
        const size = Number(u.searchParams.get('page_size') ?? '20')
        const rows = mine.slice((page - 1) * size, page * size).map((o) => ({
          order_id: o.order_id, purchased_at: o.purchased_at, status: o.status, total_centavos: total(o),
          seller_id: o.seller_id, item_count: o.items.length,
        }))
        return json(200, { orders: rows, total: mine.length, page, page_size: size })
      }
      const id = u.pathname.slice('/api/orders/'.length)
      if (!/^[0-9a-f-]{36}$/i.test(id)) return json(400, { error: { code: 'INVALID_ARGUMENT', message: `order id "${id}" is not a uuid` } })
      const o = mine.find((x) => x.order_id === id.toLowerCase())
      if (!o) return json(404, { error: { code: 'NOT_FOUND', message: 'order not found' } })
      const { owner, ...detail } = o
      void owner
      return json(200, { ...detail, total_centavos: total(o) })
    }
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
