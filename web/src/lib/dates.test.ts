import { formatSaoPaulo } from '@/lib/dates'

describe('formatSaoPaulo', () => {
  it.each([
    ['2026-11-01T10:00:00Z', '2026-11-01 07:00'],
    ['2026-11-01T02:30:59.999Z', '2026-10-31 23:30'],
    ['2017-10-02T13:56:33Z', '2017-10-02 10:56'],
    // The dataset's summers had daylight saving (UTC-2).
    ['2017-12-01T12:00:00Z', '2017-12-01 10:00'],
  ])('%s → %s', (iso, want) => {
    expect(formatSaoPaulo(iso)).toBe(want)
  })
})
