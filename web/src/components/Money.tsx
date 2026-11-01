import type React from 'react'

import { formatCentavos } from '@/lib/money'
import { cn } from '@/lib/utils'

// Money shows int64 centavos as BRL, right-aligned in the numeric style so
// columns of amounts line up (DESIGN.md, UX-DR3).
export function Money({ centavos, className, ...rest }: { centavos: number; className?: string } & Omit<React.ComponentProps<'span'>, 'children'>) {
  return (
    <span className={cn('block text-right font-mono font-medium tabular-nums', className)} {...rest}>
      {formatCentavos(centavos)}
    </span>
  )
}
