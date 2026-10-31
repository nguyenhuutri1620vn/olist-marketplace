import { ApiError, apiFetch, setUnauthorizedHandler } from '@/lib/api'
import { saveSession } from '@/lib/session'
import { tokenFor } from '@/test/gateway'

function respond(status: number, body: unknown) {
  return vi.fn(async () => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }))
}

beforeEach(() => sessionStorage.clear())
afterEach(() => {
  vi.unstubAllGlobals()
  setUnauthorizedHandler(null)
})

describe('apiFetch', () => {
  it('sends the session token and JSON body to the gateway path', async () => {
    const token = tokenFor('customer')
    saveSession(token, 'a@b.co')
    const f = respond(200, { ok: true })
    vi.stubGlobal('fetch', f)
    await expect(apiFetch('/api/orders', { method: 'POST', body: { a: 1 } })).resolves.toEqual({ ok: true })
    const [url, init] = f.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toBe('/api/orders')
    expect(new Headers(init.headers).get('Authorization')).toBe(`Bearer ${token}`)
    expect(init.body).toBe('{"a":1}')
  })

  it('turns the gateway error body into an ApiError', async () => {
    vi.stubGlobal('fetch', respond(409, { error: { code: 'ALREADY_EXISTS', message: 'email already registered' } }))
    const err = await apiFetch('/api/auth/register', { method: 'POST', body: {} }).catch((e) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ status: 409, code: 'ALREADY_EXISTS', message: 'email already registered' })
  })

  it('reports a non-JSON failure and a network failure', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('bad gateway', { status: 502 })))
    await expect(apiFetch('/api/x')).rejects.toMatchObject({ status: 502, code: 'HTTP_502' })
    vi.stubGlobal('fetch', vi.fn(async () => { throw new TypeError('Failed to fetch') }))
    await expect(apiFetch('/api/x')).rejects.toMatchObject({ status: 0, code: 'UNAVAILABLE' })
  })

  it('signals an expired session on 401 when a token was sent', async () => {
    saveSession(tokenFor('seller'), 'a@b.co')
    const onUnauthorized = vi.fn()
    setUnauthorizedHandler(onUnauthorized)
    vi.stubGlobal('fetch', respond(401, { error: { code: 'UNAUTHENTICATED', message: 'invalid or expired token' } }))
    await expect(apiFetch('/api/seller/orders')).rejects.toBeInstanceOf(ApiError)
    expect(onUnauthorized).toHaveBeenCalledTimes(1)
  })

  it('does not treat a failed login as an expired session', async () => {
    const onUnauthorized = vi.fn()
    setUnauthorizedHandler(onUnauthorized)
    vi.stubGlobal('fetch', respond(401, { error: { code: 'UNAUTHENTICATED', message: 'invalid email or password' } }))
    await expect(apiFetch('/api/auth/login', { method: 'POST', body: {} })).rejects.toBeInstanceOf(ApiError)
    expect(onUnauthorized).not.toHaveBeenCalled()
  })

  it('sends no token to the auth routes and never expires the session there', async () => {
    saveSession(tokenFor('admin'), 'admin@demo.olist.local')
    const onUnauthorized = vi.fn()
    setUnauthorizedHandler(onUnauthorized)
    const f = respond(401, { error: { code: 'UNAUTHENTICATED', message: 'invalid email or password' } })
    vi.stubGlobal('fetch', f)
    await expect(apiFetch('/api/auth/login', { method: 'POST', body: {}, auth: false })).rejects.toBeInstanceOf(ApiError)
    const [, init] = f.mock.calls[0] as unknown as [string, RequestInit]
    expect(new Headers(init.headers).has('Authorization')).toBe(false)
    expect(onUnauthorized).not.toHaveBeenCalled()
  })

  it('resolves an empty success body', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(null, { status: 204 })))
    await expect(apiFetch('/api/x', { method: 'POST' })).resolves.toBeUndefined()
  })

  it('rethrows a caller abort as is', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => { throw new DOMException('aborted', 'AbortError') }))
    await expect(apiFetch('/api/x')).rejects.toMatchObject({ name: 'AbortError' })
  })
})
