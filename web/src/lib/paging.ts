// pageParam reads ?page=, falling back to 1 for anything but a positive int32.
export function pageParam(raw: string | null): number {
  if (!raw || !/^[1-9]\d{0,9}$/.test(raw)) return 1
  const n = Number(raw)
  return n <= 2 ** 31 - 1 ? n : 1
}
