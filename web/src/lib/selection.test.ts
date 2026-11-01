import { emptySelection, groupBySeller, loadSelection, saveSelection, summarize, toggle } from '@/lib/selection'

const a1 = { product_id: 'a1'.padEnd(32, '0'), display_name: 'health_beauty #a100', category_name_en: 'health_beauty', seller_id: 'aaaa'.padEnd(32, '0'), seller_city: 'ibitinga', seller_state: 'SP', price_centavos: 1000, freight_centavos: 250 }
const a2 = { ...a1, product_id: 'a2'.padEnd(32, '0'), display_name: 'health_beauty #a200', price_centavos: 2000, freight_centavos: 0 }
const b1 = { ...a1, product_id: 'b1'.padEnd(32, '0'), display_name: 'pc_gamer #b100', seller_id: 'bbbb'.padEnd(32, '0'), seller_city: 'rio', seller_state: 'RJ', price_centavos: 500, freight_centavos: 100 }

beforeEach(() => sessionStorage.clear())

describe('selection', () => {
  it('toggles Products and changes the key with every change', () => {
    const s0 = emptySelection()
    const s1 = toggle(s0, a1)
    const s2 = toggle(s1, b1)
    expect(s2.items.map((p) => p.product_id)).toEqual([a1.product_id, b1.product_id])
    expect(new Set([s0.key, s1.key, s2.key]).size).toBe(3)
    const s3 = toggle(s2, a1)
    expect(s3.items.map((p) => p.product_id)).toEqual([b1.product_id])
    expect(s3.key).not.toBe(s2.key)
  })

  it('summarizes items, Sellers and price + freight', () => {
    expect(summarize([a1, a2, b1])).toEqual({ items: 3, sellers: 2, total: 1000 + 250 + 2000 + 500 + 100 })
  })

  it('groups by Seller with subtotals, sorted by seller id', () => {
    const groups = groupBySeller([b1, a1, a2])
    expect(groups.map((g) => [g.seller_id, g.items.length, g.subtotal])).toEqual([
      [a1.seller_id, 2, 3250],
      [b1.seller_id, 1, 600],
    ])
  })

  it('round-trips through sessionStorage with its key', () => {
    const s = toggle(toggle(emptySelection(), a1), b1)
    saveSelection(s)
    expect(loadSelection()).toEqual(s)
  })

  it.each([
    ['nothing', null],
    ['not JSON', '{nope'],
    ['wrong shape', JSON.stringify({ key: 'k', items: 'x' })],
    ['bad item', JSON.stringify({ key: 'k', items: [{ product_id: 1 }] })],
    ['no key', JSON.stringify({ items: [] })],
    ['a repeated Product', JSON.stringify({ key: 'k', items: [a1, a1] })],
    ['a negative price', JSON.stringify({ key: 'k', items: [{ ...a1, price_centavos: -1 }] })],
    ['a fractional freight', JSON.stringify({ key: 'k', items: [{ ...a1, freight_centavos: 1.5 }] })],
  ])('ignores %s in storage', (_, raw) => {
    if (raw !== null) sessionStorage.setItem('olist.selection', raw)
    expect(loadSelection().items).toEqual([])
  })
})
