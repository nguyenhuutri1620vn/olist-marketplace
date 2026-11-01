import { createContext, useContext } from 'react'

import type { SelectedProduct, Selection } from '@/lib/selection'

export type SelectionValue = {
  selection: Selection
  isSelected: (productID: string) => boolean
  toggle: (p: SelectedProduct) => void
  clear: () => void
}

export const SelectionContext = createContext<SelectionValue | null>(null)

export function useSelection(): SelectionValue {
  const v = useContext(SelectionContext)
  if (!v) throw new Error('useSelection outside SelectionProvider')
  return v
}
