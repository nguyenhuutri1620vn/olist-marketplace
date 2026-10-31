import { useState, type FormEvent, type ReactNode } from 'react'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

// AuthForm is the shared email + password form of Login and Register.
export function AuthForm({
  title,
  submit,
  passwordAutoComplete,
  onSubmit,
  children,
}: {
  title: string
  submit: string
  passwordAutoComplete: 'current-password' | 'new-password'
  onSubmit: (email: string, password: string) => Promise<void>
  children?: ReactNode
}) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)

  async function handle(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    try {
      await onSubmit(email, password)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card className="mx-auto max-w-md">
      <CardHeader>
        <CardTitle>
          <h1 className="text-xl font-semibold">{title}</h1>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <form className="space-y-4" onSubmit={handle}>
          <div className="space-y-2">
            <Label htmlFor="email">Email</Label>
            <Input id="email" type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="password">Password</Label>
            <Input
              id="password"
              type="password"
              autoComplete={passwordAutoComplete}
              required
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </div>
          <Button type="submit" disabled={busy}>
            {submit}
          </Button>
        </form>
        {children}
      </CardContent>
    </Card>
  )
}
