import { NavLink, Outlet, Link } from 'react-router'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import type { Role } from '@/lib/routes'
import { cn } from '@/lib/utils'
import { useSession } from '@/session/context'

// navLinks are each role's links (EXPERIENCE.md information architecture).
const navLinks: Record<Role | 'visitor', [label: string, to: string][]> = {
  visitor: [['Products', '/'], ['Log in', '/login']],
  customer: [['Products', '/'], ['My Orders', '/orders']],
  seller: [['Seller Orders', '/seller/orders'], ['Seller Revenue', '/seller/revenue']],
  admin: [['Ops Dashboard', '/admin'], ['Vouchers', '/admin/vouchers'], ['Payment gateway', '/admin/gateway']],
}

// Layout is the top nav bar plus one content column capped at 1152px.
export function Layout() {
  const { session, logOut } = useSession()
  const links = navLinks[session?.role ?? 'visitor']
  return (
    <div className="min-h-screen bg-background text-foreground">
      <header className="border-b">
        <nav className="mx-auto flex max-w-[1152px] items-center gap-6 px-4 py-3">
          <Link to="/" className="font-semibold">
            Olist Marketplace
          </Link>
          {links.map(([label, to]) => (
            <NavLink
              key={to}
              to={to}
              end
              className={({ isActive }) => cn('text-sm text-muted-foreground', isActive && 'text-foreground font-medium')}
            >
              {label}
            </NavLink>
          ))}
          {session && (
            <div className="ml-auto">
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="outline" size="sm">
                    {session.email}
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuLabel>{session.email}</DropdownMenuLabel>
                  <DropdownMenuLabel className="font-normal text-muted-foreground">Role: {session.role}</DropdownMenuLabel>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem onSelect={logOut}>Log out</DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          )}
        </nav>
      </header>
      <main className="mx-auto max-w-[1152px] px-4 py-6">
        <Outlet />
      </main>
    </div>
  )
}
