import { useState } from 'react'
import { RotateCcw, X } from 'lucide-react'

// RewindButton is the small control revealed on user-message hover that rewinds
// the conversation to that message: it and everything after it are removed
// (conversation-only — file changes are NOT reverted). Like DeleteButton it uses
// a two-step inline confirm (⟲ → "Geri sar" / ✕) so the action stays in the UI.
export function RewindButton({ onClick }: { onClick: () => void }) {
  const [armed, setArmed] = useState(false)
  if (armed) {
    return (
      <span className="flex shrink-0 items-center gap-1">
        <button
          onClick={() => {
            setArmed(false)
            onClick()
          }}
          title="Bu mesaj ve sonrasını sil, sohbeti buraya geri sar"
          className="rounded px-1.5 py-0.5 text-[10px] font-semibold text-[var(--color-accent)] transition hover:bg-[var(--color-accent-soft)]"
        >
          Geri sar
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
      title="Buraya geri sar (bu mesaj + sonrasını sil)"
      className="shrink-0 rounded p-0.5 text-[var(--color-text-dim)] opacity-0 transition hover:text-[var(--color-accent)] group-hover:opacity-100"
    >
      <RotateCcw size={14} />
    </button>
  )
}
