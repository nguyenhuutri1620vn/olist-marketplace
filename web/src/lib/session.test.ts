import { clearSession, decodeClaims, loadSession, saveSession } from '@/lib/session'
import { fakeJWT } from '@/test/jwt'

const now = 1_800_000_000

describe('decodeClaims', () => {
  it('reads sub, role and exp', () => {
    expect(decodeClaims(fakeJWT({ sub: 'a1', role: 'seller', exp: now }))).toEqual({ sub: 'a1', role: 'seller', exp: now })
  })
  it('rejects malformed tokens and unknown roles', () => {
    expect(decodeClaims('nope')).toBeNull()
    expect(decodeClaims(fakeJWT({ sub: 'a1', role: 'root', exp: now }))).toBeNull()
    expect(decodeClaims(fakeJWT({ sub: 'a1', role: 'admin' }))).toBeNull()
  })
})

describe('session storage', () => {
  beforeEach(() => sessionStorage.clear())

  it('round-trips a live session', () => {
    const token = fakeJWT({ sub: 'a1', role: 'admin', exp: now + 60 })
    saveSession(token, 'admin@demo.olist.local')
    expect(loadSession(now)).toEqual({ token, email: 'admin@demo.olist.local', accountId: 'a1', role: 'admin', exp: now + 60 })
  })

  it('treats an expired token as no session', () => {
    saveSession(fakeJWT({ sub: 'a1', role: 'admin', exp: now }), 'x@y.z')
    expect(loadSession(now)).toBeNull()
  })

  it('clears', () => {
    saveSession(fakeJWT({ sub: 'a1', role: 'admin', exp: now + 60 }), 'x@y.z')
    clearSession()
    expect(loadSession(now)).toBeNull()
  })
})
