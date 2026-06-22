import { useState } from 'react'
import { Trash2, X } from 'lucide-react'

// DeleteButton is the small destructive control revealed on message hover. It
// uses a two-step inline confirm (🗑 → "Sil" / ✕) instead of a blocking native
// dialog, so deleting a message stays in the UI.
export function DeleteButton({ onClick }: { onClick: () => void }) {
  const [armed, setArmed] = useState(false)
  if (armed) {
    return (
      <span className="flex shrink-0 items-center gap-1">
        <button
          onClick={() => {
            setArmed(false)
            onClick()
          }}
          className="rounded px-1.5 py-0.5 text-[10px] font-semibold text-[var(--color-danger)] transition hover:bg-[color-mix(in_srgb,var(--color-danger)_15%,transparent)]"
        >
          Sil
        </button>
        <button
          onClick={() => setArmed(false)}
          title="Vazgeç"
          className="rounded px-1 py-0.5 text-[10px] text-[var(--color-text-dim)] transition hover:text-[var(--color-text)]"
        >
          <X size={12} />
        </button>
      </span>
    )
  }
  return (
    <button
      onClick={() => setArmed(true)}
      title="Mesajı sil"
      className="shrink-0 rounded p-0.5 text-[var(--color-text-dim)] opacity-0 transition hover:text-[var(--color-danger)] group-hover:opacity-100"
    >
      <Trash2 size={14} />
    </button>
  )
}
