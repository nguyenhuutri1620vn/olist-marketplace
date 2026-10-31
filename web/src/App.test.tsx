import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { apiFetch } from '@/lib/api'
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

const nav = () => screen.getByRole('navigation')

async function logIn(email: string, password = demoPassword) {
  const user = userEvent.setup()
  await user.type(screen.getByLabelText('Email'), email)
  await user.type(screen.getByLabelText('Password'), password)
  await user.click(screen.getByRole('button', { name: 'Log in' }))
  return user
}

describe('logged out', () => {
  it('shows the brand, Products and Log in only', () => {
    renderApp('/')
    const links = within(nav()).getAllByRole('link').map((a) => a.textContent)
    expect(links).toEqual(['Olist Marketplace', 'Products', 'Log in'])
  })

  it('sends a role route to Login and back after login', async () => {
    renderApp('/admin/vouchers')
    expect(await screen.findByRole('heading', { name: 'Log in' })).toBeInTheDocument()
    expect(screen.queryByText('Session expired. Log in again.')).not.toBeInTheDocument()
    await logIn(demo.admin)
    expect(await screen.findByRole('heading', { name: 'Vouchers' })).toBeInTheDocument()
  })
})

describe('Login page', () => {
  it('has labelled fields, the demo hint and a register link', () => {
    renderApp('/login')
    expect(screen.getByLabelText('Email')).toHaveAttribute('type', 'email')
    expect(screen.getByLabelText('Password')).toHaveAttribute('type', 'password')
    const hint = screen.getByRole('region', { name: 'Demo accounts' })
    const emails = within(hint).getAllByRole('listitem').map((li) => li.textContent)
    expect(emails.filter((e) => e?.startsWith('customer-'))).toHaveLength(2)
    expect(emails.filter((e) => e?.startsWith('seller-'))).toHaveLength(2)
    expect(emails).toContain('admin@demo.olist.local')
    expect(hint).toHaveTextContent('olist-demo-2026')
    expect(hint).toHaveTextContent('README')
    expect(screen.getByRole('link', { name: /register/i })).toHaveAttribute('href', '/register')
  })

  it.each([
    ['customer', demo.customer, 'Products', ['Products', 'My Orders']],
    ['seller', demo.seller, 'Seller Orders', ['Seller Orders', 'Seller Revenue']],
    ['admin', demo.admin, 'Ops Dashboard', ['Ops Dashboard', 'Vouchers', 'Payment gateway']],
  ])('logs a %s in to its home and nav', async (role, email, home, links) => {
    renderApp('/login')
    const user = await logIn(email)
    expect(await screen.findByRole('heading', { name: home })).toBeInTheDocument()
    const navLinks = within(nav()).getAllByRole('link').map((a) => a.textContent)
    expect(navLinks).toEqual(['Olist Marketplace', ...links])
    await user.click(within(nav()).getByRole('button', { name: email }))
    expect(await screen.findByText(`Role: ${role}`)).toBeInTheDocument()
    await user.click(screen.getByRole('menuitem', { name: 'Log out' }))
    await waitFor(() => expect(within(nav()).getByRole('link', { name: 'Log in' })).toBeInTheDocument())
  })

  it('shows the gateway error verbatim on a wrong password', async () => {
    renderApp('/login')
    await logIn(demo.admin, 'wrong-password')
    expect(await screen.findByText('Request failed: UNAUTHENTICATED — invalid email or password')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Log in' })).toBeInTheDocument()
  })

  it('ignores an unsafe return-to', async () => {
    renderApp('/login?next=//evil.com/admin')
    await logIn(demo.customer)
    expect(await screen.findByRole('heading', { name: 'Products' })).toBeInTheDocument()
  })
})

describe('Register page', () => {
  async function register(email: string) {
    const user = userEvent.setup()
    await user.type(screen.getByLabelText('Email'), email)
    await user.type(screen.getByLabelText('Password'), 's3cret-pw')
    await user.click(screen.getByRole('button', { name: 'Register' }))
  }

  it('creates a Customer and logs it in', async () => {
    renderApp('/register')
    await register('new@example.com')
    expect(await screen.findByRole('heading', { name: 'Products' })).toBeInTheDocument()
    expect(within(nav()).getByRole('link', { name: 'My Orders' })).toBeInTheDocument()
    expect(within(nav()).getByRole('button', { name: 'new@example.com' })).toBeInTheDocument()
  })

  it('shows a duplicate email as the gateway says it', async () => {
    gw.registered.add('taken@example.com')
    renderApp('/register')
    await register('taken@example.com')
    expect(await screen.findByText('Request failed: ALREADY_EXISTS — email already registered')).toBeInTheDocument()
  })
})

describe('session expiry', () => {
  it('sends an expired session to Login with the message and returns after login', async () => {
    saveSession(tokenFor('customer', -10), demo.customer)
    renderApp('/orders')
    expect(await screen.findByText('Session expired. Log in again.')).toBeInTheDocument()
    await logIn(demo.customer)
    expect(await screen.findByRole('heading', { name: 'My Orders' })).toBeInTheDocument()
  })

  it('does not return to another role’s route after login', async () => {
    saveSession(tokenFor('customer', -10), demo.customer)
    renderApp('/orders')
    await screen.findByText('Session expired. Log in again.')
    await logIn(demo.seller)
    expect(await screen.findByRole('heading', { name: 'Seller Orders' })).toBeInTheDocument()
  })
})

describe('session ends while the app is open', () => {
  it('a 401 on a data call sends the user to Login and back afterwards', async () => {
    saveSession(tokenFor('customer'), demo.customer)
    renderApp('/orders')
    await screen.findByRole('heading', { name: 'My Orders' })
    await act(async () => {
      await apiFetch('/api/expired-probe').catch(() => {})
    })
    expect(await screen.findByText('Session expired. Log in again.')).toBeInTheDocument()
    expect(within(nav()).getByRole('link', { name: 'Log in' })).toBeInTheDocument()
    await logIn(demo.customer)
    expect(await screen.findByRole('heading', { name: 'My Orders' })).toBeInTheDocument()
  })

  it('reaching exp on a role route sends the user to Login', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      saveSession(tokenFor('seller', 5), demo.seller)
      renderApp('/seller/revenue')
      await screen.findByRole('heading', { name: 'Seller Revenue' })
      await act(async () => {
        vi.advanceTimersByTime(6000)
      })
      expect(await screen.findByText('Session expired. Log in again.')).toBeInTheDocument()
    } finally {
      vi.useRealTimers()
    }
  })

  it('reaching exp on a public route only logs out', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      saveSession(tokenFor('customer', 5), demo.customer)
      renderApp('/')
      await act(async () => {
        vi.advanceTimersByTime(6000)
      })
      await waitFor(() => expect(within(nav()).getByRole('link', { name: 'Log in' })).toBeInTheDocument())
      expect(screen.getByRole('heading', { name: 'Products' })).toBeInTheDocument()
    } finally {
      vi.useRealTimers()
    }
  })

  it('a wrong password while logged in keeps the session', async () => {
    saveSession(tokenFor('admin'), demo.admin)
    renderApp('/login')
    await logIn(demo.admin, 'wrong-password')
    expect(await screen.findAllByText('Request failed: UNAUTHENTICATED — invalid email or password')).not.toHaveLength(0)
    expect(screen.queryByText('Session expired. Log in again.')).not.toBeInTheDocument()
    expect(within(nav()).getByRole('button', { name: demo.admin })).toBeInTheDocument()
  })

  it('an expired token found at load is cleared, so the message shows once', async () => {
    saveSession(tokenFor('customer', -10), demo.customer)
    const first = renderApp('/orders')
    await screen.findByText('Session expired. Log in again.')
    first.unmount()
    renderApp('/orders')
    await screen.findByRole('heading', { name: 'Log in' })
    expect(screen.queryByText('Session expired. Log in again.')).not.toBeInTheDocument()
  })
})

describe('log out', () => {
  it('from a role route lands on Products, not on Login', async () => {
    saveSession(tokenFor('admin'), demo.admin)
    renderApp('/admin/vouchers')
    const user = userEvent.setup()
    await user.click(within(nav()).getByRole('button', { name: demo.admin }))
    await user.click(await screen.findByRole('menuitem', { name: 'Log out' }))
    expect(await screen.findByRole('heading', { name: 'Products' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Log in' })).not.toBeInTheDocument()
  })
})

describe('access', () => {
  it('shows the no-access page for another role’s route', async () => {
    saveSession(tokenFor('seller'), demo.seller)
    renderApp('/admin')
    expect(await screen.findByText("You don't have access to this page.")).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Go to your home page' })).toHaveAttribute('href', '/seller/orders')
  })

  it('has a JSON-free not-found page', () => {
    renderApp('/nope')
    expect(screen.getByRole('heading', { name: 'Page not found' })).toBeInTheDocument()
  })
})

describe('toasts', () => {
  it('render in an aria-live region', async () => {
    renderApp('/login')
    await logIn(demo.admin, 'wrong-password')
    // Sonner's toast store is global, so earlier tests' toasts may still be listed.
    const toasts = await screen.findAllByText(/Request failed/)
    expect(toasts[0].closest('[aria-live]')).not.toBeNull()
  })
})
