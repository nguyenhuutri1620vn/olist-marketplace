import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

// Full class names, so Tailwind keeps every one (DESIGN.md status tokens).
const fill: Record<string, string> = {
  created: 'bg-status-created',
  approved: 'bg-status-approved',
  processing: 'bg-status-processing',
  shipped: 'bg-status-shipped',
  delivered: 'bg-status-delivered',
  canceled: 'bg-status-canceled',
}

// StatusBadge is a pill filled with its status colour and the exact word in
// white; the text is always shown, never colour alone (UX-DR2).
export function StatusBadge({ status }: { status: string }) {
  const bg = fill[status]
  return <Badge className={cn(bg ? `${bg} text-white` : 'bg-muted text-foreground')}>{status}</Badge>
}
