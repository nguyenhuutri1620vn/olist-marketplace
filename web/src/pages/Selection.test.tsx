import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { saveSession } from '@/lib/session'
import { demo, demoPassword, fakeGateway, tokenFor } from '@/test/gateway'
import { renderApp } from '@/test/render'

let gw: ReturnType<typeof fakeGateway>

beforeEach(() => {
  sessionStorage.clear()
  gw = fakeGateway()
  vi.stubGlobal('fetch', vi.fn(gw.fetch))
})

afterEach(() => vi.unstubAllGlobals())

async function loaded() {
  await waitFor(() => expect(screen.queryAllByTestId('skeleton-cell')).toHaveLength(0))
}

const bar = () => screen.queryByRole('region', { name: 'Checkout selection' })

// pick ticks two health_beauty Products on page 1 and one Uncategorized.
async function pick(user: ReturnType<typeof userEvent.setup>) {
  await loaded()
  await user.click(screen.getByRole('checkbox', { name: 'Select health_beauty #0000' }))
  await user.click(screen.getByRole('checkbox', { name: 'Select health_beauty #0001' }))
  await user.click(screen.getByRole('combobox', { name: 'Category' }))
  await user.click(screen.getByRole('option', { name: 'Uncategorized' }))
  await loaded()
  await waitFor(() => expect(screen.getAllByRole('row')).toHaveLength(4))
  await user.click(screen.getByRole('checkbox', { name: 'Select Uncategorized #002d' }))
}

describe('selecting Products', () => {
  it('shows a bar with items, Sellers and price + freight across filters', async () => {
    const user = userEvent.setup()
    renderApp('/')
    await loaded()
    expect(bar()).not.toBeInTheDocument()
    await pick(user)
    // (100000 + 1000) + (100001 + 1000) + (100045 + 1000) centavos.
    expect(bar()).toHaveTextContent('3 items from 2 Sellers · R$ 3.030,46')
    expect(within(bar()!).getByRole('button', { name: 'Check out' })).toBeEnabled()
  })

  it('uses singular words for one item from one Seller', async () => {
    const user = userEvent.setup()
    renderApp('/')
    await loaded()
    await user.click(screen.getByRole('checkbox', { name: 'Select health_beauty #0000' }))
    expect(bar()).toHaveTextContent('1 item from 1 Seller · R$ 1.010,00')
    await user.click(screen.getByRole('checkbox', { name: 'Select health_beauty #0000' }))
    expect(bar()).not.toBeInTheDocument()
  })

  it('keeps ticks across pages and a reload', async () => {
    const user = userEvent.setup()
    const { unmount } = renderApp('/')
    await loaded()
    await user.click(screen.getByRole('checkbox', { name: 'Select health_beauty #0000' }))
    unmount()
    renderApp('/')
    await loaded()
    expect(screen.getByRole('checkbox', { name: 'Select health_beauty #0000' })).toBeChecked()
    expect(bar()).toHaveTextContent('1 item from 1 Seller')
  })

  it('logged out, Check out goes through Login to the review with the same selection', async () => {
    const user = userEvent.setup()
    renderApp('/')
    await pick(user)
    await user.click(within(bar()!).getByRole('button', { name: 'Check out' }))
    expect(await screen.findByRole('heading', { name: 'Log in' })).toBeInTheDocument()
    await user.type(screen.getByLabelText('Email'), demo.customer)
    await user.type(screen.getByLabelText('Password'), demoPassword)
    await user.click(screen.getByRole('button', { name: 'Log in' }))
    expect(await screen.findByText('This will create 2 Orders.')).toBeInTheDocument()
  })
})

describe('selection and sessions', () => {
  it('a logged-in Customer ticks and goes straight to the review', async () => {
    const user = userEvent.setup()
    saveSession(tokenFor('customer'), demo.customer)
    renderApp('/')
    await loaded()
    await user.click(screen.getByRole('checkbox', { name: 'Select health_beauty #0000' }))
    await user.click(within(bar()!).getByRole('button', { name: 'Check out' }))
    expect(await screen.findByText('This will create 1 Order.')).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Log in' })).not.toBeInTheDocument()
  })

  it.each(['seller', 'admin'])('shows no checkboxes or bar to an %s', async (role) => {
    sessionStorage.setItem('olist.selection', JSON.stringify({ key: 'k', items: [] }))
    saveSession(tokenFor(role), role === 'seller' ? demo.seller : demo.admin)
    renderApp('/')
    await loaded()
    expect(screen.queryAllByRole('checkbox')).toHaveLength(0)
    expect(bar()).not.toBeInTheDocument()
  })

  it('logging out clears the selection', async () => {
    const user = userEvent.setup()
    saveSession(tokenFor('customer'), demo.customer)
    renderApp('/')
    await loaded()
    await user.click(screen.getByRole('checkbox', { name: 'Select health_beauty #0000' }))
    await user.click(screen.getByRole('button', { name: demo.customer }))
    await user.click(await screen.findByRole('menuitem', { name: 'Log out' }))
    await waitFor(() => expect(bar()).not.toBeInTheDocument())
    expect(sessionStorage.getItem('olist.selection')).toBeNull()
  })

  it('two quick ticks both count', async () => {
    const user = userEvent.setup()
    renderApp('/')
    await loaded()
    const a = screen.getByRole('checkbox', { name: 'Select health_beauty #0000' })
    const b = screen.getByRole('checkbox', { name: 'Select health_beauty #0001' })
    await act(async () => {
      a.click()
      b.click()
    })
    expect(bar()).toHaveTextContent('2 items from 1 Seller')
    void user
  })
})
