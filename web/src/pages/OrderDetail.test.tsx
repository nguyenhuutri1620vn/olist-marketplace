import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { saveSession } from '@/lib/session'
import { demo, fakeGateway, tokenFor } from '@/test/gateway'
import { renderApp } from '@/test/render'

let gw: ReturnType<typeof fakeGateway>

beforeEach(() => {
  sessionStorage.clear()
  gw = fakeGateway()
  vi.stubGlobal('fetch', vi.fn(gw.fetch))
  saveSession(tokenFor('customer'), demo.customer)
})

afterEach(() => vi.unstubAllGlobals())

const id = '0192f000-0000-7000-8000-000000000100'

// headerBadge is the status badge next to the title (the timeline has more).
const headerBadge = () => within(screen.getByRole('heading', { name: 'Order 00000100' }).parentElement!).getByText(/./, { selector: '[data-slot=badge]' })

async function loaded() {
  await waitFor(() => expect(screen.queryAllByTestId('skeleton-cell')).toHaveLength(0))
  await screen.findByRole('heading', { name: 'Order 00000100' })
}

describe('Order detail', () => {
  it('shows items, total and the timeline', async () => {
    renderApp(`/orders/${id}`)
    await loaded()
    expect(headerBadge()).toHaveTextContent('created')
    expect(screen.getByText('Purchased 2026-11-01 09:00')).toBeInTheDocument()
    const rows = within(screen.getByRole('table')).getAllByRole('row').slice(1)
    expect(rows.map((r) => within(r).getAllByRole('cell').map((c) => c.textContent))).toEqual([
      ['health_beauty #aaaa', 'R$ 58,90', 'R$ 13,29'],
      ['Uncategorized #bbbb', 'R$ 7,60', 'R$ 0,00'],
    ])
    expect(screen.getByText('Total')).toBeInTheDocument()
    expect(screen.getByTestId('order-total')).toHaveTextContent('R$ 79,79')
    const timeline = within(screen.getByRole('list', { name: 'Status timeline' })).getAllByRole('listitem')
    expect(timeline.map((li) => li.textContent)).toEqual(['created2026-11-01 09:00'])
  })

  it('lists the timeline oldest first and explains a cancel', async () => {
    const o = gw.state.orders[0]
    o.status = 'canceled'
    o.cancel_reason = 'CARD_DECLINED'
    o.voucher_restored = true
    o.timeline.push({ status: 'canceled', at: '2026-11-01T12:05:00Z' })
    renderApp(`/orders/${id}`)
    await loaded()
    const timeline = within(screen.getByRole('list', { name: 'Status timeline' })).getAllByRole('listitem')
    expect(timeline.map((li) => li.textContent)).toEqual(['created2026-11-01 09:00', 'canceled2026-11-01 09:05'])
    expect(screen.getByText('Canceled: CARD_DECLINED. Voucher restored.')).toBeInTheDocument()
  })

  it('Refresh shows a status changed elsewhere', async () => {
    const user = userEvent.setup()
    renderApp(`/orders/${id}`)
    await loaded()
    const o = gw.state.orders[0]
    o.status = 'approved'
    o.timeline.push({ status: 'approved', at: '2026-11-01T12:01:00Z' })
    await user.click(screen.getByRole('button', { name: 'Refresh' }))
    await waitFor(() => expect(headerBadge()).toHaveTextContent('approved'))
  })

  it('names an Order without a cancel reason plainly', async () => {
    const o = gw.state.orders[0]
    o.status = 'canceled'
    renderApp(`/orders/${id}`)
    await loaded()
    expect(screen.getByText('Canceled.')).toBeInTheDocument()
  })

  it('shows a Voucher and what was left to pay', async () => {
    gw.state.orders[0].voucher_amount_centavos = 1000
    renderApp(`/orders/${id}`)
    await loaded()
    expect(screen.getByTestId('order-voucher')).toHaveTextContent('R$ 10,00')
    expect(screen.getByTestId('order-to-pay')).toHaveTextContent('R$ 69,79')
  })

  it('titles the Order by its canonical id', async () => {
    gw.state.orders[0].order_id = '0192f000-0000-7000-8000-00000000abcd'
    renderApp('/orders/0192F000-0000-7000-8000-00000000ABCD')
    expect(await screen.findByRole('heading', { name: 'Order 0000abcd' })).toBeInTheDocument()
  })

  it('treats a malformed id as not found, without a toast', async () => {
    renderApp('/orders/not-a-uuid')
    expect(await screen.findByText('Order not found.')).toBeInTheDocument()
    expect(screen.queryByText(/Request failed/)).not.toBeInTheDocument()
  })

  it('shows an outage as a toast, not as not found', async () => {
    gw.state.fail = ['/api/orders']
    renderApp(`/orders/${id}`)
    expect(await screen.findByText('Request failed: UNAVAILABLE — Service Unavailable')).toBeInTheDocument()
    expect(screen.queryByText('Order not found.')).not.toBeInTheDocument()
  })

  it('says Order not found for an id I cannot see', async () => {
    renderApp('/orders/0192f000-0000-7000-8000-999999999999')
    expect(await screen.findByText('Order not found.')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Back to My Orders' })).toHaveAttribute('href', '/orders')
  })
})
