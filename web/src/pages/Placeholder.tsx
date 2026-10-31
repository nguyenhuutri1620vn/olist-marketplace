// Placeholder is a role screen that later stories build out; it keeps every
// nav link and post-login route real until then.
export function Placeholder({ title }: { title: string }) {
  return (
    <div className="space-y-2">
      <h1 className="text-2xl font-semibold">{title}</h1>
      <p className="text-muted-foreground">This screen arrives with a later story.</p>
    </div>
  )
}
