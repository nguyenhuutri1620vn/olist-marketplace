const saoPaulo = new Intl.DateTimeFormat('en-CA', {
  timeZone: 'America/Sao_Paulo',
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
})

// formatSaoPaulo shows an RFC 3339 instant as `YYYY-MM-DD HH:mm` in São
// Paulo time, the dataset's time zone (EXPERIENCE.md).
export function formatSaoPaulo(iso: string): string {
  const parts = Object.fromEntries(saoPaulo.formatToParts(new Date(iso)).map((p) => [p.type, p.value]))
  return `${parts.year}-${parts.month}-${parts.day} ${parts.hour}:${parts.minute}`
}
