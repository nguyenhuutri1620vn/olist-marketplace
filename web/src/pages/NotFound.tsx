import { Link } from 'react-router'

export function NotFound() {
  return (
    <div className="space-y-2">
      <h1 className="text-2xl font-semibold">Page not found</h1>
      <Link className="underline" to="/">
        Go to Products
      </Link>
    </div>
  )
}
