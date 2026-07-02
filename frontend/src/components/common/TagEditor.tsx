import { useState, type KeyboardEvent } from 'react'
import { X } from 'lucide-react'

// TagEditor — an inline chips editor for free-form tags on sessions, flows and
// schedules. Adding a tag (Enter or comma) or removing one (×) calls onChange
// with the full new set; the parent persists it. Purely controlled — it holds no
// tags of its own, only the in-progress input text.
interface Props {
  tags: string[]
  onChange: (tags: string[]) => void
  placeholder?: string
  disabled?: boolean
  className?: string
}

export function TagEditor({ tags, onChange, placeholder = 'Etiket ekle…', disabled, className = '' }: Props) {
  const [draft, setDraft] = useState('')

  const commit = () => {
    const t = draft.trim().replace(/,+$/, '').trim()
    setDraft('')
    if (!t || tags.includes(t)) return
    onChange([...tags, t])
  }

  const remove = (tag: string) => onChange(tags.filter((t) => t !== tag))

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault()
      commit()
    } else if (e.key === 'Backspace' && draft === '' && tags.length > 0) {
      // Backspace on an empty box removes the last tag (fast correction).
      remove(tags[tags.length - 1])
    }
  }

  return (
    <div
      className={`flex flex-wrap items-center gap-1 rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1.5 ${className}`}
    >
      {tags.map((tag) => (
        <span
          key={tag}
          className="inline-flex items-center gap-1 rounded bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-[11px] font-medium text-[var(--color-accent)]"
        >
          {tag}
          {!disabled && (
            <button
              type="button"
              onClick={() => remove(tag)}
              className="opacity-70 hover:opacity-100"
              aria-label={`${tag} etiketini kaldır`}
            >
              <X size={11} />
            </button>
          )}
        </span>
      ))}
      {!disabled && (
        <input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={onKeyDown}
          onBlur={commit}
          placeholder={tags.length === 0 ? placeholder : ''}
          className="min-w-[80px] flex-1 bg-transparent text-[12px] text-[var(--color-text)] outline-none placeholder:text-[var(--color-text-dim)]"
        />
      )}
    </div>
  )
}
