import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router'

import { CheckoutBar } from '@/components/CheckoutBar'
import { Money } from '@/components/Money'
import { Pager } from '@/components/Pager'
import { TableSkeleton } from '@/components/TableSkeleton'
import { toastRequestError } from '@/components/requestError'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { apiFetch } from '@/lib/api'
import { type SelectedProduct, pick } from '@/lib/selection'
import { useSelection } from '@/selection/context'
import { useSession } from '@/session/context'
import { pageParam } from '@/lib/paging'

// PAGE_SIZE is the Products table page (UX-DR8).
const PAGE_SIZE = 20

// ALL is the filter value for every Category; Radix Select reserves "".
const ALL = 'all'

type Category = { name_pt: string; name_en: string }

// A list row is what the selection keeps, plus the Category's dataset name.
type ProductRow = SelectedProduct & { category_name_pt: string }

type ProductPage = { products: ProductRow[]; total: number }

// A result belongs to the query it answered; a result for another query
// means the current one is still loading.
type Result = { query: string; page: ProductPage | null }

// sellerLabel tells Sellers apart without a name: the first 8 characters of
// the id, as in the demo emails, and the location.
function sellerLabel(p: Pick<ProductRow, 'seller_id' | 'seller_city' | 'seller_state'>): string {
  return `${p.seller_id.slice(0, 8)} · ${p.seller_city}, ${p.seller_state}`
}

// Products lists the catalog 20 per page with a Category filter (FR13). The
// filter and page live in the URL, so reload and Back keep them.
export function Products() {
  const [params, setParams] = useSearchParams()
  const { session } = useSession()
  const { isSelected, toggle } = useSelection()
  // Visitors and Customers pick Products for Checkout; Sellers and Admins browse.
  const selectable = !session || session.role === 'customer'
  const columns = selectable ? 5 : 4
  const category = params.get('category') ?? ''
  const page = pageParam(params.get('page'))

  const [categories, setCategories] = useState<Category[]>([])
  const [result, setResult] = useState<Result | null>(null)

  useEffect(() => {
    const ctl = new AbortController()
    apiFetch<{ categories: Category[] }>('/api/categories', { signal: ctl.signal })
      .then((r) => setCategories(r.categories))
      .catch((e: unknown) => {
        if (!ctl.signal.aborted) toastRequestError(e)
      })
    return () => ctl.abort()
  }, [])

  const q = new URLSearchParams()
  if (category) q.set('category', category)
  q.set('page', String(page))
  q.set('page_size', String(PAGE_SIZE))
  const query = q.toString()

  useEffect(() => {
    const ctl = new AbortController()
    apiFetch<ProductPage>(`/api/products?${query}`, { signal: ctl.signal })
      .then((r) => setResult({ query, page: r }))
      .catch((e: unknown) => {
        if (ctl.signal.aborted) return
        setResult({ query, page: null })
        toastRequestError(e)
      })
    return () => ctl.abort()
  }, [query])

  const loading = result?.query !== query
  const data = loading ? null : result.page
  // The page count is known only once this query has answered.
  const pages = data ? Math.max(1, Math.ceil(data.total / PAGE_SIZE)) : null
  // A Category in the URL that is not (yet) in the list still shows.
  const unlisted = category && !categories.some((c) => c.name_pt === category) ? category : null

  function go(next: { category?: string; page?: number }) {
    const p = new URLSearchParams()
    const c = next.category ?? category
    if (c) p.set('category', c)
    const n = next.page ?? 1
    if (n > 1) p.set('page', String(n))
    setParams(p)
  }

  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-semibold">Products</h1>
      <div className="flex items-center gap-2">
        <Label htmlFor="category-filter">Category</Label>
        <Select value={category || ALL} onValueChange={(v) => go({ category: v === ALL ? '' : v })}>
          <SelectTrigger id="category-filter" className="w-64">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>All Categories</SelectItem>
            {unlisted && <SelectItem value={unlisted}>{unlisted}</SelectItem>}
            {categories.map((c) => (
              <SelectItem key={c.name_pt} value={c.name_pt}>
                {c.name_en}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            {selectable && (
              <TableHead className="w-8">
                <span className="sr-only">Select</span>
              </TableHead>
            )}
            <TableHead>Name</TableHead>
            <TableHead>Category</TableHead>
            <TableHead>Seller</TableHead>
            <TableHead className="text-right">Price</TableHead>
          </TableRow>
        </TableHeader>
        {loading ? (
          <TableSkeleton columns={columns} />
        ) : (
          <TableBody>
            {data?.products.map((p) => (
              <TableRow key={p.product_id}>
                {selectable && (
                  <TableCell>
                    <Checkbox aria-label={`Select ${p.display_name}`} checked={isSelected(p.product_id)} onCheckedChange={() => toggle(pick(p))} />
                  </TableCell>
                )}
                <TableCell>{p.display_name}</TableCell>
                <TableCell>{p.category_name_en}</TableCell>
                <TableCell>{sellerLabel(p)}</TableCell>
                <TableCell>
                  <Money centavos={p.price_centavos} />
                </TableCell>
              </TableRow>
            ))}
            {data && data.products.length === 0 && (
              <TableRow>
                <TableCell colSpan={columns} className="text-muted-foreground">
                  {data.total > 0 ? 'This page is past the last one.' : category ? 'No Products in this Category.' : 'No Products yet.'}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        )}
      </Table>
      <Pager page={page} pages={pages} loading={loading} onGo={(n) => go({ page: n })} />
      {selectable && <CheckoutBar />}
    </div>
  )
}
