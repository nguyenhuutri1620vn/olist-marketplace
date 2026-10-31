import { startTransition, useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useLocation, useNavigate } from 'react-router'

import { setUnauthorizedHandler } from '@/lib/api'
import { loginURL, roleOfPath } from '@/lib/routes'
import { clearSession, loadSession, saveSession, storedSessionExpired, type Session } from '@/lib/session'
import { SessionContext, type SessionValue } from '@/session/context'


export function SessionProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(() => loadSession())
  const [expired, setExpired] = useState(() => {
    // Clear a token that expired while the tab was closed, so the message
    // shows on the next role route only.
    const was = storedSessionExpired()
    if (was) clearSession()
    return was
  })
  const navigate = useNavigate()
  const location = useLocation()
  const here = useRef('')
  here.current = location.pathname + location.search + location.hash

  // The session ends on a gateway 401 or when the token reaches exp. On a
  // role route that means Login with "Session expired"; on a public route
  // the visitor just becomes logged out (UX-DR6).
  const expire = useCallback(() => {
    clearSession()
    const onRoleRoute = roleOfPath(here.current.split(/[?#]/)[0]) !== null
    startTransition(() => {
      if (onRoleRoute) navigate(loginURL(here.current, true), { replace: true })
      setSession(null)
      setExpired(onRoleRoute)
    })
  }, [navigate])

  useEffect(() => {
    setUnauthorizedHandler(expire)
    return () => setUnauthorizedHandler(null)
  }, [expire])

  useEffect(() => {
    if (!session) return
    // setTimeout overflows past 2^31-1 ms; tokens live 1 h anyway.
    const ms = Math.min(Math.max(session.exp * 1000 - Date.now(), 0), 2_147_483_647)
    const t = setTimeout(expire, ms)
    return () => clearTimeout(t)
  }, [session, expire])

  const value = useMemo<SessionValue>(
    () => ({
      session,
      expired,
      logIn: (token, email) => {
        saveSession(token, email)
        const s = loadSession()
        setSession(s)
        setExpired(false)
        return s
      },
      logOut: () => {
        clearSession()
        // The router applies navigations in a transition; clearing the
        // session in the same transition keeps the role route's guard from
        // seeing "logged out" there and redirecting to Login.
        startTransition(() => {
          navigate('/', { replace: true })
          setSession(null)
          setExpired(false)
        })
      },
    }),
    [session, expired, navigate],
  )
  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>
}
