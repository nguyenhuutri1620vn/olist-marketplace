import { render, screen } from '@testing-library/react'

import { StatusBadge } from '@/components/StatusBadge'
import { statuses } from '@/lib/orders'

describe('StatusBadge', () => {
  it('lists the six dataset words', () => {
    expect(statuses).toEqual(['created', 'approved', 'processing', 'shipped', 'delivered', 'canceled'])
  })

  it.each(['created', 'approved', 'processing', 'shipped', 'delivered', 'canceled'])(
    '%s is a pill filled with its token, white exact word',
    (status) => {
      render(<StatusBadge status={status} />)
      const badge = screen.getByText(status)
      expect(badge).toHaveAttribute('data-slot', 'badge')
      expect(badge).toHaveClass(`bg-status-${status}`, 'text-white', 'rounded-4xl')
    },
  )

  it('still shows an unknown word, uncoloured', () => {
    render(<StatusBadge status="lost" />)
    expect(screen.getByText('lost')).not.toHaveClass('text-white')
  })
})
