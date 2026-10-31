import { render } from '@testing-library/react'
import { MemoryRouter } from 'react-router'

import { AppRoutes, Providers } from '@/App'

// renderApp renders the whole app at path.
export function renderApp(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Providers>
        <AppRoutes />
      </Providers>
    </MemoryRouter>,
  )
}
