import { Button } from '@/components/ui/button'

// Pager is the Previous/Next row of every paginated table (no infinite
// scroll). pages is null until the current query has answered.
export function Pager({ page, pages, loading, onGo }: { page: number; pages: number | null; loading: boolean; onGo: (page: number) => void }) {
  return (
    <div className="flex items-center justify-end gap-2">
      <span className="text-sm text-muted-foreground">
        Page {page}
        {pages !== null && ` of ${pages}`}
      </span>
      <Button variant="outline" disabled={loading || page <= 1} onClick={() => onGo(Math.min(page - 1, pages ?? page - 1))}>
        Previous
      </Button>
      <Button variant="outline" disabled={pages === null || page >= pages} onClick={() => onGo(page + 1)}>
        Next
      </Button>
    </div>
  )
}
