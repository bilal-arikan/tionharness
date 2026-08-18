import { useState } from 'react'
import { RotateCcw, X } from 'lucide-react'
import { actionChip, actionChipActive } from './messageActions'

// RewindButton rewinds the conversation to a user message: it and everything after
// it are removed (conversation-only — file changes are NOT reverted). Like
// DeleteButton it uses a two-step inline confirm (⟲ → "Geri sar" / ✕) so the
// action stays in the UI. Visible at rest, in the turn footer.
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
          className={actionChipActive('default', 'font-semibold')}
        >
          Geri sar
        </button>
        <button
          onClick={() => setArmed(false)}
          title="Vazgeç"
          aria-label="Vazgeç"
          className={actionChip()}
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
      aria-label="Buraya geri sar"
      className={actionChip()}
    >
      <RotateCcw size={13} />
    </button>
  )
}
