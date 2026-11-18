import { useEffect, useRef, useState, type FocusEvent, type KeyboardEvent } from 'react'
import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { TagEditor } from '@/shared/components'

interface Props {
  tags: string[]
  onChange: (tags: string[]) => void | Promise<boolean | void>
  // Placeholder of the revealed input (TagEditor's default when omitted).
  placeholder?: string
}

// CardTags is the tag row of a board card. Existing tags render through the
// shared TagEditor exactly as before; a card WITHOUT tags shows only a small
// "+ tag" text action instead of an empty input box, so a lane of untagged rules
// is not a column of blank fields. The action reveals the editor and focuses its
// input; Escape or leaving the field while it is still empty folds it again.
export function CardTags({ tags, onChange, placeholder }: Props) {
  const { t } = useTranslation('schedules')
  const [open, setOpen] = useState(false)
  const boxRef = useRef<HTMLDivElement>(null)
  const addRef = useRef<HTMLButtonElement>(null)
  // Set when the editor is folded from the keyboard, so focus returns to the
  // action instead of falling back to <body>.
  const refocusAdd = useRef(false)
  const expanded = open || tags.length > 0

  useEffect(() => {
    if (open) {
      boxRef.current?.querySelector('input')?.focus()
    } else if (refocusAdd.current) {
      refocusAdd.current = false
      addRef.current?.focus()
    }
  }, [open])

  const draftOf = (target: EventTarget) =>
    target instanceof HTMLInputElement ? target.value.trim() : ''

  const onBlur = (e: FocusEvent<HTMLDivElement>) => {
    // Focus moving between the input and a tag's remove button stays inside.
    if (e.currentTarget.contains(e.relatedTarget)) return
    // A typed draft is being committed by TagEditor's own blur handler; keep the
    // editor mounted so a failed save does not lose it.
    if (draftOf(e.target) === '') setOpen(false)
  }

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== 'Escape' || tags.length > 0 || draftOf(e.target) !== '') return
    e.stopPropagation()
    refocusAdd.current = true
    setOpen(false)
  }

  if (!expanded) {
    return (
      <button
        ref={addRef}
        type="button"
        onClick={() => setOpen(true)}
        title={t('common.addTag')}
        aria-label={t('common.addTag')}
        data-testid="card-tags-add"
        className="inline-flex h-9 items-center gap-1 rounded text-[11px] text-[var(--color-text-dim)] transition hover:text-[var(--color-text)] md:h-6"
      >
        <Plus size={11} className="shrink-0" />
        {t('common.addTagShort')}
      </button>
    )
  }

  return (
    <div ref={boxRef} onBlur={onBlur} onKeyDown={onKeyDown} data-testid="card-tags-editor">
      <TagEditor tags={tags} onChange={onChange} placeholder={placeholder} className="py-1" />
    </div>
  )
}
