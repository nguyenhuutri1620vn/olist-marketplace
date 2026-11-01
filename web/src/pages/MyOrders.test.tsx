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

const bodyRows = () => within(screen.getByRole('table')).getAllByRole('row').slice(1)
const orderCalls = () => gw.calls.map((c) => c.url).filter((u) => u.startsWith('/api/orders?'))

async function loaded() {
  await waitFor(() => expect(screen.queryAllByTestId('skeleton-cell')).toHaveLength(0))
}

describe('My Orders', () => {
  it('lists my Orders newest first, 20 per page', async () => {
    renderApp('/orders')
    expect(screen.getByRole('heading', { name: 'My Orders' })).toBeInTheDocument()
    expect(screen.getAllByTestId('skeleton-cell').length).toBeGreaterThan(0)
    await loaded()
    const headers = within(screen.getByRole('table')).getAllByRole('columnheader').map((h) => h.textContent)
    expect(headers).toEqual(['Date', 'Order', 'Seller', 'Total', 'Status'])
    expect(bodyRows()).toHaveLength(20)
    const first = within(bodyRows()[0]).getAllByRole('cell').map((c) => c.textContent)
    expect(first).toEqual(['2026-11-01 09:00', '00000100', '6560211a', 'R$ 79,79', 'created'])
    expect(within(bodyRows()[0]).getByRole('link', { name: '00000100' })).toHaveAttribute(
      'href',
      '/orders/0192f000-0000-7000-8000-000000000100',
    )
    expect(screen.getByText('Page 1 of 2')).toBeInTheDocument()
    expect(orderCalls()).toEqual(['/api/orders?page=1&page_size=20'])
  })

  it('pages through the URL', async () => {
    const user = userEvent.setup()
    renderApp('/orders')
    await loaded()
    await user.click(screen.getByRole('button', { name: 'Next' }))
    expect(await screen.findByText('Page 2 of 2')).toBeInTheDocument()
    await loaded()
    expect(bodyRows()).toHaveLength(5)
    expect(orderCalls().at(-1)).toBe('/api/orders?page=2&page_size=20')
  })

  it('Refresh shows a status changed elsewhere', async () => {
    const user = userEvent.setup()
    renderApp('/orders')
    await loaded()
    gw.state.orders[0].status = 'approved'
    expect(within(bodyRows()[0]).getByText('created')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Refresh' }))
    expect(await within(bodyRows()[0]).findByText('approved')).toBeInTheDocument()
    expect(orderCalls()).toHaveLength(2)
  })

  it('says when the page in the URL is past the last one', async () => {
    const user = userEvent.setup()
    renderApp('/orders?page=9')
    expect(await screen.findByText('This page is past the last one.')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Previous' }))
    expect(await screen.findByText('Page 2 of 2')).toBeInTheDocument()
  })

  it.each(['0', 'abc'])('treats page=%s as page 1', async (page) => {
    renderApp(`/orders?page=${page}`)
    await loaded()
    expect(orderCalls()).toEqual(['/api/orders?page=1&page_size=20'])
  })

  it('says so when I have no Orders', async () => {
    gw.state.orders = []
    renderApp('/orders')
    expect(await screen.findByText('No Orders yet.')).toBeInTheDocument()
  })

  it('shows a failure as a toast', async () => {
    gw.state.fail = ['/api/orders']
    renderApp('/orders')
    expect(await screen.findByText('Request failed: UNAVAILABLE — Service Unavailable')).toBeInTheDocument()
  })
})
