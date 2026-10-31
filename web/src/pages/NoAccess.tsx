import { Link } from 'react-router'

import { homeFor } from '@/lib/routes'
import { useSession } from '@/session/context'

export function NoAccess() {
  const { session } = useSession()
  return (
    <div className="space-y-2">
      <p>You don't have access to this page.</p>
      <Link className="underline" to={session ? homeFor(session.role) : '/'}>
        Go to your home page
      </Link>
    </div>
  )
}
