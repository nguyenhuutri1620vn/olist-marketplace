import { Link, useNavigate } from 'react-router'

import { toastRequestError } from '@/components/requestError'
import { apiFetch } from '@/lib/api'
import { homeFor } from '@/lib/routes'
import { AuthForm } from '@/pages/AuthForm'
import type { TokenResponse } from '@/pages/Login'
import { useSession } from '@/session/context'

// Register creates a Customer Account and logs it in with the returned token.
export function Register() {
  const navigate = useNavigate()
  const { logIn } = useSession()

  async function submit(email: string, password: string) {
    try {
      const t = await apiFetch<TokenResponse>('/api/auth/register', { method: 'POST', body: { email, password }, auth: false })
      const s = logIn(t.access_token, email)
      if (!s) throw new Error('the gateway returned an unusable token')
      navigate(homeFor(s.role), { replace: true })
    } catch (err) {
      toastRequestError(err)
    }
  }

  return (
    <AuthForm title="Register" submit="Register" passwordAutoComplete="new-password" onSubmit={submit}>
      <p className="text-sm">
        Have an account?{' '}
        <Link className="underline" to="/login">
          Log in
        </Link>
      </p>
    </AuthForm>
  )
}
