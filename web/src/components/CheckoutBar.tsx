import { useNavigate } from 'react-router'

import { Money } from '@/components/Money'
import { Button } from '@/components/ui/button'
import { summarize } from '@/lib/selection'
import { loginURL } from '@/lib/routes'
import { useSelection } from '@/selection/context'
import { useSession } from '@/session/context'

function plural(n: number, word: string): string {
  return `${n} ${word}${n === 1 ? '' : 's'}`
}

// CheckoutBar is the sticky summary of the selection: "N items from M
// Sellers · R$ total · [Check out]" (UX-DR9). Logged out, Check out goes to
// Login and back to the review.
export function CheckoutBar() {
  const { selection } = useSelection()
  const { session } = useSession()
  const navigate = useNavigate()
  if (selection.items.length === 0) return null
  const s = summarize(selection.items)
  return (
    <section
      aria-label="Checkout selection"
      className="sticky bottom-0 flex items-center justify-end gap-4 border-t bg-background py-3"
    >
      <span>
        {plural(s.items, 'item')} from {plural(s.sellers, 'Seller')} ·{' '}
        <Money centavos={s.total} className="inline" />
      </span>
      <Button onClick={() => navigate(session ? '/checkout' : loginURL('/checkout', false))}>Check out</Button>
    </section>
  )
}
