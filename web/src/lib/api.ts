import { loadSession } from '@/lib/session'

// ApiError is a failed gateway call: HTTP status plus the gateway's
// `{"error":{"code","message"}}` body (Conventions).
export class ApiError extends Error {
  readonly status: number
  readonly code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

let onUnauthorized: (() => void) | null = null

// setUnauthorizedHandler registers what happens when the gateway rejects the
// session's token (401): the app sends the user to Login (UX-DR6).
export function setUnauthorizedHandler(h: (() => void) | null): void {
  onUnauthorized = h
}

type Options = {
  method?: string
  body?: unknown
  signal?: AbortSignal
  // auth: false sends no token and never ends the session on a 401 (login
  // and register, where a 401 means wrong credentials).
  auth?: boolean
}

// apiFetch calls an api-gateway route (same origin, `/api/...`) with the
// session's token and returns the JSON body.
export async function apiFetch<T>(path: string, opts: Options = {}): Promise<T> {
  const session = opts.auth === false ? null : loadSession()
  const headers = new Headers({ Accept: 'application/json' })
  if (opts.body !== undefined) headers.set('Content-Type', 'application/json')
  if (session) headers.set('Authorization', `Bearer ${session.token}`)

  let resp: Response
  try {
    resp = await fetch(path, {
      method: opts.method ?? 'GET',
      headers,
      body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
      signal: opts.signal,
    })
  } catch (e) {
    if (e instanceof DOMException && e.name === 'AbortError') throw e
    throw new ApiError(0, 'UNAVAILABLE', e instanceof Error ? e.message : 'network error')
  }
  if (resp.ok) {
    const text = await resp.text()
    return (text ? JSON.parse(text) : undefined) as T
  }

  let code = `HTTP_${resp.status}`
  let message = resp.statusText || 'request failed'
  try {
    const body = (await resp.json()) as { error?: { code?: string; message?: string } }
    if (body.error?.code) code = body.error.code
    if (body.error?.message) message = body.error.message
  } catch {
    // Not the gateway's JSON body (e.g. a proxy error page).
  }
  if (resp.status === 401 && session) onUnauthorized?.()
  throw new ApiError(resp.status, code, message)
}
