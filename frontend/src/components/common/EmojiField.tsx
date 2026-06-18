import { useState } from 'react'
import { Smile } from 'lucide-react'
import { EmojiPicker } from '../agents/EmojiPicker'

interface Props {
  /** Currently selected emoji ('' = none / default). */
  value: string
  onChange: (emoji: string) => void
  /** Glyph shown in the trigger when no emoji is selected (e.g. "Aa", "⬡"). */
  clearLabel?: string
  /**
   * Optional descriptive text shown next to the glyph instead of the compact
   * Smile icon. When a function, it receives the current value so callers can
   * vary the wording (e.g. "Emoji seç" vs "Emojiyi değiştir").
   */
  label?: string | ((value: string) => string)
}

// EmojiField is the single, reusable emoji/icon control used everywhere a user
// picks an emoji (agent avatars, workspace icons, …): a trigger button showing
// the current selection plus the shared EmojiPicker popover. Centralizing it
// here keeps one consistent emoji UI across the app.
export function EmojiField({ value, onChange, clearLabel = 'Aa', label }: Props) {
  const [open, setOpen] = useState(false)
  const text = typeof label === 'function' ? label(value) : label
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
        {text ? (
          <span className="text-[var(--color-text-dim)]">{text}</span>
        ) : (
          <Smile size={14} className="text-[var(--color-text-dim)]" />
        )}
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
