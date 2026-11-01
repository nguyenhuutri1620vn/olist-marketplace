import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router'

import { Money } from '@/components/Money'
import { StatusBadge } from '@/components/StatusBadge'
import { TableSkeleton } from '@/components/TableSkeleton'
import { toastRequestError } from '@/components/requestError'
import { Button } from '@/components/ui/button'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { ApiError, apiFetch } from '@/lib/api'
import { formatSaoPaulo } from '@/lib/dates'
import { type OrderDetail as Detail, itemName, orderRef } from '@/lib/orders'

type Result = { key: string; order: Detail | null; notFound: boolean }

// OrderDetail shows one of the Customer's Orders: items, total and the
// status timeline oldest first, with Refresh and no live push (UX-DR12).
export function OrderDetail() {
  const { id = '' } = useParams()
  const [refreshes, setRefreshes] = useState(0)
  const [result, setResult] = useState<Result | null>(null)
  const key = `${id}#${refreshes}`

  useEffect(() => {
    const ctl = new AbortController()
    apiFetch<Detail>(`/api/orders/${encodeURIComponent(id)}`, { signal: ctl.signal })
      .then((order) => setResult({ key, order, notFound: false }))
      .catch((e: unknown) => {
        if (ctl.signal.aborted) return
        // 404 and 400 both mean this is not one of my Orders.
        const notFound = e instanceof ApiError && (e.status === 404 || e.status === 400)
        setResult({ key, order: null, notFound })
        if (!notFound) toastRequestError(e)
      })
    return () => ctl.abort()
  }, [id, key])

  const loading = result?.key !== key
  const order = loading ? null : result.order

  if (!loading && result.notFound) {
    return (
      <div className="space-y-2">
        <p>Order not found.</p>
        <Link className="underline" to="/orders">
          Back to My Orders
        </Link>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-semibold">
            Order <span className="font-mono">{orderRef(order?.order_id ?? id)}</span>
          </h1>
          {order && <StatusBadge status={order.status} />}
        </div>
        <Button variant="outline" onClick={() => setRefreshes((n) => n + 1)}>
          Refresh
        </Button>
      </div>
      {order && <p className="text-muted-foreground">Purchased {formatSaoPaulo(order.purchased_at)}</p>}
      {order?.status === 'canceled' && (
        <p>
          {order.cancel_reason ? `Canceled: ${order.cancel_reason}.` : 'Canceled.'}
          {order.voucher_restored ? ' Voucher restored.' : ''}
        </p>
      )}
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Item</TableHead>
            <TableHead className="text-right">Price</TableHead>
            <TableHead className="text-right">Freight</TableHead>
          </TableRow>
        </TableHeader>
        {loading ? (
          <TableSkeleton columns={3} />
        ) : (
          <TableBody>
            {order?.items.map((it, i) => (
              <TableRow key={`${it.product_id}-${i}`}>
                <TableCell>{itemName(it)}</TableCell>
                <TableCell>
                  <Money centavos={it.price_centavos} />
                </TableCell>
                <TableCell>
                  <Money centavos={it.freight_centavos} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        )}
      </Table>
      {order && (
        <>
          <div className="flex justify-end gap-4">
            <span className="font-medium">Total</span>
            <Money centavos={order.total_centavos} className="w-32" data-testid="order-total" />
          </div>
          {order.voucher_amount_centavos > 0 && (
            <>
              <div className="flex justify-end gap-4">
                <span>Voucher</span>
                <Money centavos={order.voucher_amount_centavos} className="w-32" data-testid="order-voucher" />
              </div>
              <div className="flex justify-end gap-4">
                <span className="font-medium">Card</span>
                <Money centavos={order.total_centavos - order.voucher_amount_centavos} className="w-32" data-testid="order-to-pay" />
              </div>
            </>
          )}
          <section className="space-y-2">
            <h2 className="text-lg font-semibold">Status timeline</h2>
            <ol aria-label="Status timeline" className="space-y-1">
              {order.timeline.map((c) => (
                <li key={c.status} className="flex items-center gap-3">
                  <StatusBadge status={c.status} />
                  <span className="text-sm text-muted-foreground">{formatSaoPaulo(c.at)}</span>
                </li>
              ))}
            </ol>
          </section>
        </>
      )}
    </div>
  )
}
