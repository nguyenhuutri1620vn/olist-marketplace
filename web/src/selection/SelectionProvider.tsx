import { type ReactNode, useEffect, useMemo, useRef, useState } from 'react'

import { type SelectedProduct, type Selection, emptySelection, loadSelection, saveSelection, toggle as toggled } from '@/lib/selection'
import { SelectionContext } from '@/selection/context'
import { useSession } from '@/session/context'

// SelectionProvider holds the Checkout selection in page state and mirrors
// every change to sessionStorage (UX-DR9). A visitor who logs in keeps it,
// and so does a Customer whose session expires (Login returns to the
// review); logging out or another account logging in clears it, so the next
// person in the tab never inherits it.
export function SelectionProvider({ children }: { children: ReactNode }) {
  const [selection, setSelection] = useState<Selection>(loadSelection)
  const { session, expired } = useSession()
  const account = session?.accountId ?? null
  const previous = useRef(account)

  useEffect(() => {
    const before = previous.current
    previous.current = account
    const loggedOut = before !== null && account === null && !expired
    const switched = before !== null && account !== null && before !== account
    if (loggedOut || switched) {
      const next = emptySelection()
      saveSelection(next)
      setSelection(next)
    }
  }, [account, expired])

  const value = useMemo(
    () => ({
      selection,
      isSelected: (id: string) => selection.items.some((p) => p.product_id === id),
      // Functional updates: two toggles before a render both count.
      toggle: (p: SelectedProduct) =>
        setSelection((prev) => {
          const next = toggled(prev, p)
          saveSelection(next)
          return next
        }),
      clear: () => {
        const next = emptySelection()
        saveSelection(next)
        setSelection(next)
      },
    }),
    [selection],
  )
  return <SelectionContext.Provider value={value}>{children}</SelectionContext.Provider>
}
