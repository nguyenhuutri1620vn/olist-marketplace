import { createContext, useContext } from 'react'

import type { Session } from '@/lib/session'

export type SessionValue = {
  session: Session | null
  // expired is true when the last session ended by expiry or a 401.
  expired: boolean
  logIn: (token: string, email: string) => Session | null
  logOut: () => void
}

export const SessionContext = createContext<SessionValue | null>(null)

export function useSession(): SessionValue {
  const v = useContext(SessionContext)
  if (!v) throw new Error('useSession outside SessionProvider')
  return v
}
