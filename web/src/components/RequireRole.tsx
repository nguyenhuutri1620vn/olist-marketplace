import { Navigate, Outlet, useLocation } from 'react-router'

import type { Role } from '@/lib/routes'
import { NoAccess } from '@/pages/NoAccess'
import { loginURL } from '@/lib/routes'
import { useSession } from '@/session/context'

// RequireRole guards a role's routes in the UI. The gateway's refusal stays
// the real guard (FR-16); this only picks the right page.
export function RequireRole({ role }: { role: Role }) {
  const { session, expired } = useSession()
  const location = useLocation()
  if (!session) return <Navigate to={loginURL(location.pathname + location.search + location.hash, expired)} replace />
  if (session.role !== role) return <NoAccess />
  return <Outlet />
}
