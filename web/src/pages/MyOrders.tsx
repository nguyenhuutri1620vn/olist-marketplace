import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router'

import { Money } from '@/components/Money'
import { Pager } from '@/components/Pager'
import { StatusBadge } from '@/components/StatusBadge'
import { TableSkeleton } from '@/components/TableSkeleton'
import { toastRequestError } from '@/components/requestError'
import { Button } from '@/components/ui/button'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { apiFetch } from '@/lib/api'
import { pageParam } from '@/lib/paging'
import { formatSaoPaulo } from '@/lib/dates'
import { type OrderSummary, orderRef, shortID } from '@/lib/orders'

const PAGE_SIZE = 20

type OrderPage = { orders: OrderSummary[]; total: number }

// A result belongs to the query and refresh it answered; any other one
// means the current request is still loading.
type Result = { key: string; page: OrderPage | null }

// MyOrders lists the Customer's Orders newest first, 20 per page, with a
// Refresh button and no live push (FR7, UX-DR11).
export function MyOrders() {
  const [params, setParams] = useSearchParams()
  const page = pageParam(params.get('page'))
  const [refreshes, setRefreshes] = useState(0)
  const [result, setResult] = useState<Result | null>(null)

  const query = new URLSearchParams({ page: String(page), page_size: String(PAGE_SIZE) }).toString()
  const key = `${query}#${refreshes}`

  useEffect(() => {
    const ctl = new AbortController()
    apiFetch<OrderPage>(`/api/orders?${query}`, { signal: ctl.signal })
      .then((r) => setResult({ key, page: r }))
      .catch((e: unknown) => {
        if (ctl.signal.aborted) return
        setResult({ key, page: null })
        toastRequestError(e)
      })
    return () => ctl.abort()
  }, [key, query])

  const loading = result?.key !== key
  const data = loading ? null : result.page
  const pages = data ? Math.max(1, Math.ceil(data.total / PAGE_SIZE)) : null

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">My Orders</h1>
        <Button variant="outline" onClick={() => setRefreshes((n) => n + 1)}>
          Refresh
        </Button>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Date</TableHead>
            <TableHead>Order</TableHead>
            <TableHead>Seller</TableHead>
            <TableHead className="text-right">Total</TableHead>
            <TableHead>Status</TableHead>
          </TableRow>
        </TableHeader>
        {loading ? (
          <TableSkeleton columns={5} />
        ) : (
          <TableBody>
            {data?.orders.map((o) => (
              <TableRow key={o.order_id}>
                <TableCell>{formatSaoPaulo(o.purchased_at)}</TableCell>
                <TableCell>
                  <Link className="font-mono underline" to={`/orders/${o.order_id}`}>
                    {orderRef(o.order_id)}
                  </Link>
                </TableCell>
                <TableCell className="font-mono">{shortID(o.seller_id)}</TableCell>
                <TableCell>
                  <Money centavos={o.total_centavos} />
                </TableCell>
                <TableCell>
                  <StatusBadge status={o.status} />
                </TableCell>
              </TableRow>
            ))}
            {data && data.orders.length === 0 && (
              <TableRow>
                <TableCell colSpan={5} className="text-muted-foreground">
                  {data.total > 0 ? 'This page is past the last one.' : 'No Orders yet.'}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        )}
      </Table>
      <Pager page={page} pages={pages} loading={loading} onGo={(n) => setParams(n > 1 ? { page: String(n) } : {})} />
    </div>
  )
}
