import { useState, type KeyboardEvent } from 'react'
import { X } from 'lucide-react'

// TagEditor — an inline chips editor for free-form tags on sessions, flows and
// schedules. Adding a tag (Enter or comma) or removing one (×) calls onChange
// with the full new set; the parent persists it. Purely controlled — it holds no
// tags of its own, only the in-progress input text.
interface Props {
  tags: string[]
  // May persist asynchronously: the editor awaits it and keeps the typed draft
  // when it resolves to false, so a failed save does not eat the tag.
  onChange: (tags: string[]) => void | Promise<boolean | void>
  placeholder?: string
  disabled?: boolean
  className?: string
}

export function TagEditor({
  tags,
  onChange,
  placeholder = 'Etiket ekle…',
  disabled,
  className = '',
}: Props) {
  const [draft, setDraft] = useState('')
  // A save is in flight: the input is locked so the same tag cannot be committed
  // twice (Enter then blur), and the draft survives a failed save.
  const [saving, setSaving] = useState(false)

  const commit = async () => {
    if (saving) return
    const t = draft.trim().replace(/,+$/, '').trim()
    // Empty or duplicate: nothing to persist, just drop the draft.
    if (!t || tags.includes(t)) {
      setDraft('')
      return
    }
    setSaving(true)
    try {
      const ok = await onChange([...tags, t])
      if (ok !== false) setDraft('')
    } finally {
      setSaving(false)
    }
  }

  const remove = (tag: string) => onChange(tags.filter((t) => t !== tag))

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault()
      void commit()
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
          onBlur={() => void commit()}
          disabled={saving}
          placeholder={tags.length === 0 ? placeholder : ''}
          className="min-w-[80px] flex-1 bg-transparent text-[12px] text-[var(--color-text)] outline-none placeholder:text-[var(--color-text-dim)]"
        />
      )}
    </div>
  )
}
