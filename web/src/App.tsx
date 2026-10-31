import type { ReactNode } from 'react'
import { BrowserRouter, Route, Routes } from 'react-router'

import { Layout } from '@/components/Layout'
import { RequireRole } from '@/components/RequireRole'
import { Toaster } from '@/components/ui/sonner'
import { Login } from '@/pages/Login'
import { NotFound } from '@/pages/NotFound'
import { Placeholder } from '@/pages/Placeholder'
import { Register } from '@/pages/Register'
import { SessionProvider } from '@/session/SessionContext'

// Providers hold the session and the toast region; they need a router above.
export function Providers({ children }: { children: ReactNode }) {
  return (
    <SessionProvider>
      {children}
      <Toaster richColors position="top-right" theme="light" />
    </SessionProvider>
  )
}

// AppRoutes maps every surface in EXPERIENCE.md; role screens are placeholders
// until their stories land.
export function AppRoutes() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<Placeholder title="Products" />} />
        <Route path="login" element={<Login />} />
        <Route path="register" element={<Register />} />
        <Route element={<RequireRole role="customer" />}>
          <Route path="checkout" element={<Placeholder title="Checkout" />} />
          <Route path="orders" element={<Placeholder title="My Orders" />} />
          <Route path="orders/:id" element={<Placeholder title="Order" />} />
        </Route>
        <Route element={<RequireRole role="seller" />}>
          <Route path="seller/orders" element={<Placeholder title="Seller Orders" />} />
          <Route path="seller/revenue" element={<Placeholder title="Seller Revenue" />} />
        </Route>
        <Route element={<RequireRole role="admin" />}>
          <Route path="admin" element={<Placeholder title="Ops Dashboard" />} />
          <Route path="admin/vouchers" element={<Placeholder title="Vouchers" />} />
          <Route path="admin/gateway" element={<Placeholder title="Payment gateway" />} />
        </Route>
        <Route path="*" element={<NotFound />} />
      </Route>
    </Routes>
  )
}

export default function App() {
  return (
    <BrowserRouter>
      <Providers>
        <AppRoutes />
      </Providers>
    </BrowserRouter>
  )
}
