import { ArrowRight } from 'lucide-react'

// DirectionBadge renders the "→ <recipient>" direction cue for a turn in a
// multi-participant thread (generic participant model). The recipient label is
// resolved by the caller from Message.recipientId — an agent name, or "herkes"
// for a broadcast ("*"). Rendered only when a specific recipient is present; a
// turn addressed to the thread at large (empty recipientId) shows nothing.
export function DirectionBadge({ label }: { label?: string }) {
  if (!label) return null
  return (
    <span
      title={`Bu mesaj ${label} adresli`}
      className="inline-flex items-center gap-0.5 rounded-full border border-[var(--color-border)] px-1.5 py-0.5 text-[10px] text-[var(--color-text-dim)]"
    >
      <ArrowRight size={10} />
      {label}
    </span>
  )
}
