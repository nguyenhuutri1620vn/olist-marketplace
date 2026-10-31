// fakeJWT builds an unsigned token with the given payload; the browser never
// verifies signatures, it only reads role and expiry.
export function fakeJWT(payload: Record<string, unknown>): string {
  const b64 = (o: unknown) => btoa(JSON.stringify(o)).replace(/=+$/, '').replace(/\+/g, '-').replace(/\//g, '_')
  return `${b64({ alg: 'RS256', kid: 'k' })}.${b64(payload)}.sig`
}

