import { useEffect, useMemo, useRef, useState } from 'react'
import { useOutsideClick } from '@/shared/hooks/useOutsideClick'
import { EMOJI_CATEGORIES, searchEmojis, type EmojiEntry } from '@/shared/lib/emojiData'

interface Props {
  /** Currently selected emoji (empty string = none / auto initials). */
  value: string
  /** Called with the chosen emoji, or '' when the user clears the selection. */
  onSelect: (emoji: string) => void
  /** Dismiss the popover without selecting. */
  onClose: () => void
}

// EmojiPicker is a self-contained popover that lets the user pick an agent
// avatar emoji from a curated, categorized list with substring search. It is
// dependency-free (no emoji-mart) to keep the bundle lean. The caller positions
// it relative to the trigger button; this component handles outside-click and
// Escape dismissal, category tabs, and search.
export function EmojiPicker({ value, onSelect, onClose }: Props) {
  const [query, setQuery] = useState('')
  const [cat, setCat] = useState(EMOJI_CATEGORIES[0].id)
  // Dismiss on outside-click (the popover is always open while mounted).
  const rootRef = useOutsideClick<HTMLDivElement>(onClose)
  const inputRef = useRef<HTMLInputElement>(null)

  // Focus the search box on mount; dismiss on Escape.
  useEffect(() => {
    inputRef.current?.focus()
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation()
        onClose()
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])

  const searching = query.trim().length > 0
  const emojis: EmojiEntry[] = useMemo(() => {
    if (searching) return searchEmojis(query)
    return EMOJI_CATEGORIES.find((c) => c.id === cat)?.emojis ?? []
  }, [query, searching, cat])

  return (
    <div
      ref={rootRef}
      className="absolute z-50 mt-2 w-72 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl"
      onClick={(e) => e.stopPropagation()}
    >
      {/* Search box */}
      <div className="border-b border-[var(--color-border)] p-2">
        <input
          ref={inputRef}
          data-testid="emoji-search-input"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Emoji ara…"
          className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
        />
      </div>

      {/* Category tabs (hidden while searching) */}
      {!searching && (
        <div className="flex items-center gap-1 border-b border-[var(--color-border)] px-2 py-1.5">
          {EMOJI_CATEGORIES.map((c) => (
            <button
              key={c.id}
              data-testid="emoji-category"
              data-category={c.id}
              onClick={() => setCat(c.id)}
              title={c.label}
              className={`flex h-7 w-7 items-center justify-center rounded text-base ${
                cat === c.id
                  ? 'bg-[var(--color-surface-2)] ring-1 ring-[var(--color-accent)]'
                  : 'hover:bg-[var(--color-surface-2)]'
              }`}
            >
              {c.icon}
            </button>
          ))}
        </div>
      )}

      {/* Emoji grid */}
      <div className="max-h-52 overflow-y-auto p-2">
        {emojis.length === 0 ? (
          <p className="py-6 text-center text-xs text-[var(--color-text-dim)]">Sonuç yok</p>
        ) : (
          <div className="grid grid-cols-8 gap-0.5">
            {emojis.map((e) => (
              <button
                key={e.char}
                data-testid="emoji-pick"
                data-emoji={e.char}
                onClick={() => onSelect(e.char)}
                title={e.keywords}
                className={`flex h-8 w-8 items-center justify-center rounded text-lg hover:bg-[var(--color-surface-2)] ${
                  value === e.char ? 'ring-1 ring-[var(--color-accent)]' : ''
                }`}
              >
                {e.char}
              </button>
            ))}
          </div>
        )}
      </div>

      {/* Footer: clear selection (auto initials) */}
      <div className="border-t border-[var(--color-border)] p-2">
        <button
          data-testid="emoji-clear"
          onClick={() => onSelect('')}
          className={`w-full rounded px-2 py-1.5 text-xs ${
            value === ''
              ? 'bg-[var(--color-surface-2)] text-[var(--color-text)]'
              : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]'
          }`}
        >
          Emoji yok — baş harf kullan (Aa)
        </button>
      </div>
    </div>
  )
}
