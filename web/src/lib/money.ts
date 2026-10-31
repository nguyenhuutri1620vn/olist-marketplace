// formatCentavos renders int64 centavos as BRL, e.g. 123456 → "R$ 1.234,56".
// It uses integer math so large amounts never lose precision (UX-DR3).
export function formatCentavos(centavos: number): string {
  const sign = centavos < 0 ? '-' : ''
  const abs = Math.abs(Math.trunc(centavos))
  const reais = Math.floor(abs / 100).toString().replace(/\B(?=(\d{3})+(?!\d))/g, '.')
  const cents = (abs % 100).toString().padStart(2, '0')
  return `${sign}R$ ${reais},${cents}`
}
