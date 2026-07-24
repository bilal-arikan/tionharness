import { useState } from 'react'
import { Trash2, X } from 'lucide-react'
import { actionChip, actionChipActive } from './messageActions'

// DeleteButton is the destructive per-message control. It uses a two-step inline
// confirm (🗑 → "Sil" / ✕) instead of a blocking native dialog, so deleting a
// message stays in the UI. Visible at rest (it lives in the turn footer, not as a
// hover-only ghost).
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
          className={actionChipActive('danger', 'font-semibold')}
        >
          Sil
        </button>
        <button onClick={() => setArmed(false)} title="Vazgeç" aria-label="Vazgeç" className={actionChip()}>
          <X size={12} />
        </button>
      </span>
    )
  }
  return (
    <button
      onClick={() => setArmed(true)}
      title="Mesajı sil"
      aria-label="Mesajı sil"
      className={actionChip('danger')}
    >
      <Trash2 size={13} />
    </button>
  )
}
