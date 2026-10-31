import { Skeleton } from '@/components/ui/skeleton'
import { TableBody, TableCell, TableRow } from '@/components/ui/table'

// TableSkeleton is the loading state of every table: 5 skeleton rows (UX-DR20).
export function TableSkeleton({ columns }: { columns: number }) {
  return (
    <TableBody>
      {Array.from({ length: 5 }, (_, r) => (
        <TableRow key={r}>
          {Array.from({ length: columns }, (_, c) => (
            <TableCell key={c}>
              <Skeleton data-testid="skeleton-cell" className="h-4 w-full" />
            </TableCell>
          ))}
        </TableRow>
      ))}
    </TableBody>
  )
}
