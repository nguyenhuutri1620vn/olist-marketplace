import { formatCentavos } from '@/lib/money'

describe('formatCentavos', () => {
  it.each([
    [123456, 'R$ 1.234,56'],
    [0, 'R$ 0,00'],
    [5, 'R$ 0,05'],
    [100, 'R$ 1,00'],
    [123456789, 'R$ 1.234.567,89'],
    [-2500, '-R$ 25,00'],
  ])('%d → %s', (centavos, want) => {
    expect(formatCentavos(centavos)).toBe(want)
  })
})
