import { useState } from 'react'
import { Smile } from 'lucide-react'
import { EmojiPicker } from '../agents/EmojiPicker'

interface Props {
  /** Currently selected emoji ('' = none / default). */
  value: string
  onChange: (emoji: string) => void
  /** Glyph shown in the trigger when no emoji is selected (e.g. "Aa", "⬡"). */
  clearLabel?: string
}

// EmojiField is the self-contained, reusable emoji control: a trigger button
// showing the current selection plus the shared EmojiPicker popover. It wraps
// the same picker used by the agent avatar editor so workspace icons and agent
// avatars share one consistent emoji UI (the task's "emoji listesinden seçim").
export function EmojiField({ value, onChange, clearLabel = 'Aa' }: Props) {
  const [open, setOpen] = useState(false)
  return (
    <div className="relative inline-block">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        title="Emoji seç"
        className="flex items-center gap-2 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2.5 py-1.5 text-sm hover:border-[var(--color-accent)]"
      >
        <span className="flex h-7 w-7 items-center justify-center rounded-full bg-[var(--color-surface-2)] text-lg leading-none">
          {value || clearLabel}
        </span>
        <Smile size={14} className="text-[var(--color-text-dim)]" />
      </button>
      {open && (
        <EmojiPicker
          value={value}
          onSelect={(emoji) => {
            onChange(emoji)
            setOpen(false)
          }}
          onClose={() => setOpen(false)}
        />
      )}
    </div>
  )
}
