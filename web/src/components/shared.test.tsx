import { render, screen } from '@testing-library/react'

import { Money } from '@/components/Money'
import { TableSkeleton } from '@/components/TableSkeleton'
import { requestErrorMessage } from '@/components/requestError'
import { ApiError } from '@/lib/api'

describe('Money', () => {
  it('renders BRL right-aligned in the numeric style', () => {
    render(<Money centavos={123456} />)
    const el = screen.getByText('R$ 1.234,56')
    expect(el).toHaveClass('text-right', 'font-mono', 'tabular-nums')
  })
})

describe('TableSkeleton', () => {
  it('renders 5 skeleton rows', () => {
    render(
      <table>
        <TableSkeleton columns={3} />
      </table>,
    )
    expect(screen.getAllByRole('row')).toHaveLength(5)
    expect(screen.getAllByTestId('skeleton-cell')).toHaveLength(15)
  })
})

describe('requestErrorMessage', () => {
  it('keeps the gateway code and message verbatim', () => {
    expect(requestErrorMessage(new ApiError(503, 'UNAVAILABLE', 'Service Unavailable'))).toBe('Request failed: UNAVAILABLE — Service Unavailable')
    expect(requestErrorMessage(new Error('boom'))).toBe('Request failed: UNKNOWN — boom')
  })
})
