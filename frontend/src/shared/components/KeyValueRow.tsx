// KeyValueRow is a compact label/value line: a dimmed label on the left and the
// value on the right, both baseline-aligned. Used in inspector cards where a
// stack of these renders a small metadata table.
export function KeyValueRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between py-0.5 text-xs">
      <span className="text-[var(--color-text-dim)]">{label}</span>
      <span className="text-[var(--color-text)]">{value}</span>
    </div>
  )
}
