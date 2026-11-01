import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { type SelectedProduct, saveSelection } from '@/lib/selection'
import { saveSession } from '@/lib/session'
import { demo, fakeCatalog, fakeGateway, tokenFor } from '@/test/gateway'
import { renderApp } from '@/test/render'

let gw: ReturnType<typeof fakeGateway>

beforeEach(() => {
  sessionStorage.clear()
  gw = fakeGateway()
  vi.stubGlobal('fetch', vi.fn(gw.fetch))
  saveSession(tokenFor('customer'), demo.customer)
})

afterEach(() => vi.unstubAllGlobals())

// select stores a selection of the fake catalog's Products at indexes.
function select(...indexes: number[]): SelectedProduct[] {
  const all = fakeCatalog()
  const items = indexes.map((i) => all[i])
  saveSelection({ key: 'key-1', items })
  return items
}

const checkoutCalls = () => gw.calls.filter((c) => c.url === '/api/checkout')

describe('Checkout review', () => {
  it('groups the selection into one Card per Seller with subtotals', async () => {
    select(0, 1, 45)
    renderApp('/checkout')
    expect(await screen.findByText('This will create 2 Orders.')).toBeInTheDocument()
    const cards = screen.getAllByRole('region', { name: /^Seller / })
    expect(cards.map((c) => c.getAttribute('aria-label'))).toEqual(['Seller 6560211a · ibitinga, SP', 'Seller 7c67e144 · sao paulo, SP'])
    const rows = within(cards[0]).getAllByRole('row').slice(1)
    expect(rows.map((r) => within(r).getAllByRole('cell').map((c) => c.textContent))).toEqual([
      ['health_beauty #0000', 'R$ 1.000,00', 'R$ 10,00'],
      ['health_beauty #0001', 'R$ 1.000,01', 'R$ 10,00'],
    ])
    expect(within(cards[0]).getByTestId('subtotal')).toHaveTextContent('R$ 2.020,01')
    expect(within(cards[1]).getByTestId('subtotal')).toHaveTextContent('R$ 1.010,45')
    expect(screen.getByTestId('grand-total')).toHaveTextContent('R$ 3.030,46')
  })

  it('uses singular words for one Order', async () => {
    select(0)
    renderApp('/checkout')
    expect(await screen.findByText('This will create 1 Order.')).toBeInTheDocument()
  })

  it.each([
    ['nothing stored', () => {}],
    ['an empty selection', () => saveSelection({ key: 'k', items: [] })],
    ['corrupt storage', () => sessionStorage.setItem('olist.selection', '{nope')],
  ])('sends %s back to Products with a toast', async (_, setup) => {
    setup()
    renderApp('/checkout')
    expect(await screen.findByText('Select at least one Product to check out.')).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: 'Products' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Place Checkout' })).not.toBeInTheDocument()
  })

  it('places the Checkout once, even on a double click, then lands on My Orders', async () => {
    const user = userEvent.setup()
    select(0, 1, 45)
    gw.state.checkoutDelayMs = 100 // the second click lands while the first is in flight
    renderApp('/checkout')
    const place = await screen.findByRole('button', { name: 'Place Checkout' })
    await user.dblClick(place)
    expect(await screen.findByText('2 Orders created — one per Seller.')).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: 'My Orders' })).toBeInTheDocument()
    expect(checkoutCalls()).toHaveLength(1)
    expect(JSON.parse(String(checkoutCalls()[0].init?.body))).toEqual({
      product_ids: [fakeCatalog()[0].product_id, fakeCatalog()[1].product_id, fakeCatalog()[45].product_id],
      checkout_idempotency_key: 'key-1',
    })
    expect(sessionStorage.getItem('olist.selection')).toBeNull()
    // Clearing the selection after success is not an empty Checkout.
    expect(screen.queryByText('Select at least one Product to check out.')).not.toBeInTheDocument()
  })

  it('keeps the selection when the session expires mid-Checkout, and returns to it after login', async () => {
    const user = userEvent.setup()
    select(0, 45)
    gw.state.fail = ['/api/checkout#401']
    renderApp('/checkout')
    await user.click(await screen.findByRole('button', { name: 'Place Checkout' }))
    expect(await screen.findByText('Session expired. Log in again.')).toBeInTheDocument()
    expect(sessionStorage.getItem('olist.selection')).not.toBeNull()
    gw.state.fail = []
    await user.type(screen.getByLabelText('Email'), demo.customer)
    await user.type(screen.getByLabelText('Password'), 'olist-demo-2026')
    await user.click(screen.getByRole('button', { name: 'Log in' }))
    expect(await screen.findByText('This will create 2 Orders.')).toBeInTheDocument()
  })

  it('says one Order for a one-Seller Checkout', async () => {
    const user = userEvent.setup()
    select(0)
    renderApp('/checkout')
    await user.click(await screen.findByRole('button', { name: 'Place Checkout' }))
    expect(await screen.findByText('1 Order created.')).toBeInTheDocument()
  })

  it('keeps the selection and its key after a failure, so a retry sends the same key', async () => {
    const user = userEvent.setup()
    select(0, 45)
    gw.state.fail = ['/api/checkout']
    renderApp('/checkout')
    await user.click(await screen.findByRole('button', { name: 'Place Checkout' }))
    expect(await screen.findByText(/Request failed: UNAVAILABLE/)).toBeInTheDocument()
    expect(screen.getByText('This will create 2 Orders.')).toBeInTheDocument()
    gw.state.fail = []
    await user.click(screen.getByRole('button', { name: 'Place Checkout' }))
    await screen.findByRole('heading', { name: 'My Orders' })
    const keys = checkoutCalls().map((c) => JSON.parse(String(c.init?.body)).checkout_idempotency_key)
    expect(keys).toEqual(['key-1', 'key-1'])
  })

  it('shows the new Orders on top of My Orders', async () => {
    const user = userEvent.setup()
    select(0, 45)
    renderApp('/checkout')
    await user.click(await screen.findByRole('button', { name: 'Place Checkout' }))
    await screen.findByRole('heading', { name: 'My Orders' })
    await waitFor(() => expect(screen.queryAllByTestId('skeleton-cell')).toHaveLength(0))
    const rows = within(screen.getByRole('table')).getAllByRole('row').slice(1)
    expect(within(rows[0]).getByRole('link').getAttribute('href')).toMatch(/^\/orders\/0192f001-/)
  })
})
