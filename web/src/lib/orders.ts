// statuses are the Order Status words exactly as the dataset spells them.
export const statuses = ['created', 'approved', 'processing', 'shipped', 'delivered', 'canceled'] as const

export type OrderSummary = {
  order_id: string
  purchased_at: string
  status: string
  total_centavos: number
  seller_id: string
  item_count: number
}

export type OrderItem = {
  product_id: string
  category_name_en: string
  price_centavos: number
  freight_centavos: number
}

export type OrderDetail = {
  order_id: string
  purchased_at: string
  status: string
  total_centavos: number
  seller_id: string
  voucher_amount_centavos: number
  items: OrderItem[]
  timeline: { status: string; at: string }[]
  cancel_reason: string
  voucher_restored: boolean
}

// itemName names an item like the catalog names Products (FR13).
export function itemName(it: Pick<OrderItem, 'product_id' | 'category_name_en'>): string {
  return `${it.category_name_en} #${it.product_id.slice(0, 4)}`
}

// shortID is the first 8 characters of a dataset id (random, enough to tell
// Sellers apart).
export function shortID(id: string): string {
  return id.slice(0, 8)
}

// orderRef is the last 8 characters of an Order id: UUIDv7 ids start with
// their timestamp, so Orders made together share their first characters.
export function orderRef(id: string): string {
  return id.slice(-8)
}
