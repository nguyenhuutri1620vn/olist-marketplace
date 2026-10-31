import { Link, useNavigate, useSearchParams } from 'react-router'

import { toastRequestError } from '@/components/requestError'
import { apiFetch } from '@/lib/api'
import { safeReturn } from '@/lib/routes'
import { AuthForm } from '@/pages/AuthForm'
import { useSession } from '@/session/context'

export type TokenResponse = { access_token: string; token_type: string; expires_in: number }

// demoAccounts is the Login hint: two seeded emails per role plus the Admin;
// the README lists all 41 (FR15).
const demoAccounts = [
  'customer-8d50f5ea@demo.olist.local',
  'customer-3e43e610@demo.olist.local',
  'seller-6560211a@demo.olist.local',
  'seller-4a3ca931@demo.olist.local',
  'admin@demo.olist.local',
]

export function Login() {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const { logIn } = useSession()

  async function submit(email: string, password: string) {
    try {
      const t = await apiFetch<TokenResponse>('/api/auth/login', { method: 'POST', body: { email, password }, auth: false })
      const s = logIn(t.access_token, email)
      if (!s) throw new Error('the gateway returned an unusable token')
      navigate(safeReturn(params.get('next'), s.role), { replace: true })
    } catch (err) {
      toastRequestError(err)
    }
  }

  return (
    <div className="space-y-4">
      {params.get('expired') === '1' && (
        <p role="status" className="mx-auto max-w-md text-sm text-destructive">
          Session expired. Log in again.
        </p>
      )}
      <AuthForm title="Log in" submit="Log in" passwordAutoComplete="current-password" onSubmit={submit}>
        <p className="text-sm">
          No account?{' '}
          <Link className="underline" to="/register">
            Register as a Customer
          </Link>
        </p>
        <section aria-labelledby="demo-accounts" className="space-y-1 rounded-md border p-3 text-sm">
          <h2 id="demo-accounts" className="font-medium">
            Demo accounts
          </h2>
          <ul className="font-mono">
            {demoAccounts.map((e) => (
              <li key={e}>{e}</li>
            ))}
          </ul>
          <p>
            Password: <span className="font-mono">olist-demo-2026</span>. The README lists every demo account.
          </p>
        </section>
      </AuthForm>
    </div>
  )
}
