import { useEffect, useRef, useState } from 'react'
import { Navigate, useNavigate } from 'react-router'
import { toast } from 'sonner'

import { Money } from '@/components/Money'
import { toastRequestError } from '@/components/requestError'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { apiFetch } from '@/lib/api'
import { shortID } from '@/lib/orders'
import { groupBySeller, summarize } from '@/lib/selection'
import { useSelection } from '@/selection/context'

type CheckoutResponse = { checkout_id: string; orders: { order_id: string }[] }

function orders(n: number): string {
  return `${n} Order${n === 1 ? '' : 's'}`
}

// CheckoutReview confirms the selection, one Card per Seller (one Order
// each), and places the Checkout with the selection's idempotency key
// (UX-DR10).
export function CheckoutReview() {
  const { selection, clear } = useSelection()
  const navigate = useNavigate()
  const [placing, setPlacing] = useState(false)
  // A ref, not state: a second click in the same tick must see it.
  const inFlight = useRef(false)
  const empty = selection.items.length === 0

  // A selection cleared by a successful Checkout is not an empty one.
  useEffect(() => {
    // The id keeps StrictMode's second effect run from showing it twice.
    if (empty && !placing) toast.error('Select at least one Product to check out.', { id: 'empty-checkout' })
  }, [empty, placing])

  if (empty && !placing) return <Navigate to="/" replace />

  const groups = groupBySeller(selection.items)
  const total = summarize(selection.items).total

  async function place() {
    if (inFlight.current || empty) return
    inFlight.current = true
    setPlacing(true)
    try {
      const resp = await apiFetch<CheckoutResponse>('/api/checkout', {
        method: 'POST',
        body: { product_ids: selection.items.map((p) => p.product_id), checkout_idempotency_key: selection.key },
      })
      const n = resp?.orders?.length ?? 0
      toast.success(n === 1 ? '1 Order created.' : `${n} Orders created — one per Seller.`)
      navigate('/orders')
      clear()
    } catch (e) {
      toastRequestError(e)
      inFlight.current = false
      setPlacing(false)
    }
  }

  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-semibold">Checkout</h1>
      <p>This will create {orders(groups.length)}.</p>
      {groups.map((g) => {
        const label = `${shortID(g.seller_id)} · ${g.seller_city}, ${g.seller_state}`
        return (
          <Card key={g.seller_id} role="region" aria-label={`Seller ${label}`}>
            <CardHeader>
              <CardTitle>Seller {label}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Item</TableHead>
                    <TableHead className="text-right">Price</TableHead>
                    <TableHead className="text-right">Freight</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {g.items.map((p) => (
                    <TableRow key={p.product_id}>
                      <TableCell>{p.display_name}</TableCell>
                      <TableCell>
                        <Money centavos={p.price_centavos} />
                      </TableCell>
                      <TableCell>
                        <Money centavos={p.freight_centavos} />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
              <div className="flex justify-end gap-4">
                <span>Subtotal</span>
                <Money centavos={g.subtotal} className="w-32" data-testid="subtotal" />
              </div>
            </CardContent>
          </Card>
        )
      })}
      <div className="flex items-center justify-end gap-4">
        <span className="font-medium">Total</span>
        <Money centavos={total} className="w-32" data-testid="grand-total" />
        <Button onClick={place} disabled={placing || empty}>
          Place Checkout
        </Button>
      </div>
    </div>
  )
}
