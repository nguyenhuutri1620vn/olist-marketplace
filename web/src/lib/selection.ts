// The Checkout selection (UX-DR9): Products picked on the Products page,
// kept in page state and mirrored to sessionStorage, so it survives a reload
// and the login redirect. There is no server-side cart.

export type SelectedProduct = {
  product_id: string
  display_name: string
  category_name_en: string
  seller_id: string
  seller_city: string
  seller_state: string
  price_centavos: number
  freight_centavos: number
}

// key is the checkout_idempotency_key: one per selection, replaced whenever
// the selection changes, so a retry or a double click sends the same key.
export type Selection = { key: string; items: SelectedProduct[] }

const storageKey = 'olist.selection'

function newKey(): string {
  return crypto.randomUUID()
}

export function emptySelection(): Selection {
  return { key: newKey(), items: [] }
}

// toggle adds p, or removes it if it is already selected.
export function toggle(s: Selection, p: SelectedProduct): Selection {
  const has = s.items.some((x) => x.product_id === p.product_id)
  return { key: newKey(), items: has ? s.items.filter((x) => x.product_id !== p.product_id) : [...s.items, p] }
}

const fields: [keyof SelectedProduct, 'string' | 'number'][] = [
  ['product_id', 'string'],
  ['display_name', 'string'],
  ['category_name_en', 'string'],
  ['seller_id', 'string'],
  ['seller_city', 'string'],
  ['seller_state', 'string'],
  ['price_centavos', 'number'],
  ['freight_centavos', 'number'],
]

function isProduct(v: unknown): v is SelectedProduct {
  if (typeof v !== 'object' || v === null) return false
  const r = v as Record<string, unknown>
  return (
    fields.every(([f, t]) => typeof r[f] === t) &&
    [r.price_centavos, r.freight_centavos].every((n) => Number.isInteger(n) && (n as number) >= 0)
  )
}

// pick keeps exactly the SelectedProduct fields, so nothing else is stored.
export function pick(p: SelectedProduct): SelectedProduct {
  return Object.fromEntries(fields.map(([f]) => [f, p[f]])) as SelectedProduct
}

// loadSelection reads the stored selection; anything unreadable is an empty one.
export function loadSelection(): Selection {
  try {
    const v = JSON.parse(sessionStorage.getItem(storageKey) ?? 'null') as { key?: unknown; items?: unknown }
    if (v && typeof v.key === 'string' && v.key && Array.isArray(v.items) && v.items.every(isProduct)) {
      const ids = new Set(v.items.map((p) => p.product_id))
      if (ids.size === v.items.length) return { key: v.key, items: v.items.map(pick) }
    }
  } catch {
    // Not JSON: ignore it.
  }
  return emptySelection()
}

export function saveSelection(s: Selection): void {
  if (s.items.length === 0) sessionStorage.removeItem(storageKey)
  else sessionStorage.setItem(storageKey, JSON.stringify(s))
}

export function lineTotal(p: SelectedProduct): number {
  return p.price_centavos + p.freight_centavos
}

// summarize counts items and Sellers and adds price + freight, as an Order's
// total does (AD-15).
export function summarize(items: SelectedProduct[]): { items: number; sellers: number; total: number } {
  return {
    items: items.length,
    sellers: new Set(items.map((p) => p.seller_id)).size,
    total: items.reduce((t, p) => t + lineTotal(p), 0),
  }
}

export type SellerGroup = { seller_id: string; seller_city: string; seller_state: string; items: SelectedProduct[]; subtotal: number }

// groupBySeller is the review's one Card per Seller (one Order each), sorted
// by seller id like the Orders Checkout returns.
export function groupBySeller(items: SelectedProduct[]): SellerGroup[] {
  const groups = new Map<string, SellerGroup>()
  for (const p of items) {
    const g = groups.get(p.seller_id) ?? { seller_id: p.seller_id, seller_city: p.seller_city, seller_state: p.seller_state, items: [], subtotal: 0 }
    g.items.push(p)
    g.subtotal += lineTotal(p)
    groups.set(p.seller_id, g)
  }
  return [...groups.values()].sort((a, b) => a.seller_id.localeCompare(b.seller_id))
}
