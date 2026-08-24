// Inline validation message. Split out so useFieldErrors stays a component-free
// module (fast refresh).
export function FieldError({ message }: { message?: string }) {
  if (!message) return null
  return <p className="mt-1 text-xs text-[var(--color-danger)]">{message}</p>
}
