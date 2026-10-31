import { toast } from 'sonner'

import { ApiError } from '@/lib/api'

// requestErrorMessage is the failure toast text, with the gateway's code and
// message verbatim (UX-DR20).
export function requestErrorMessage(err: unknown): string {
  if (err instanceof ApiError) return `Request failed: ${err.code} — ${err.message}`
  return `Request failed: UNKNOWN — ${err instanceof Error ? err.message : String(err)}`
}

// toastRequestError shows a destructive toast; the page stays usable.
export function toastRequestError(err: unknown): void {
  toast.error(requestErrorMessage(err))
}
