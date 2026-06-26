import { useState } from 'react'
import { Pencil, ChevronDown, ChevronRight } from 'lucide-react'
import type { TurnStep } from '../../types'
import { DiffView } from '../markdown/DiffView'

interface Props {
  step: TurnStep
  onOpenFile?: (path: string) => void
}

// A friendly verb for the mutating tool that produced this diff. The tool name is
// lower-cased first so it matches whether the step reports "Edit"/"Write" (the
// shared, claude-cli-style names) or a namespaced/legacy variant.
function actionLabel(step: TurnStep): string {
  if (step.created) return 'Oluştur'
  const base = (step.tool || '').toLowerCase()
  if (base === 'edit' || base === 'edit_file') return 'Düzenle'
  if (base === 'write' || base === 'write_file') return 'Yaz'
  return 'Değişiklik'
}

// DiffCard renders a single file mutation (Write / Edit) as a compact
// row: ✏️ icon, action label, clickable path and the +added/−removed line
// counts — expandable to the full unified patch. Mirrors the file-change cards
// in External Agent / Claude Code chat.
export function DiffCard({ step, onOpenFile }: Props) {
  const [open, setOpen] = useState(false)
  const path = step.path || ''
  const added = step.added || 0
  const removed = step.removed || 0
  const hasPatch = !!step.patch?.trim()

  return (
    <div className="overflow-hidden rounded-lg bg-[var(--color-surface)]">
      <button
        onClick={() => setOpen((o) => hasPatch ? !o : o)}
        className={`flex w-full items-center gap-2 rounded-lg px-3 py-1.5 text-left text-xs ${
          hasPatch ? 'hover:bg-[var(--color-surface-2)]' : 'cursor-default'
        }`}
      >
        <Pencil size={14} className="shrink-0 text-[var(--color-text-dim)]" />
        <span className="shrink-0 font-medium text-[var(--color-text)]">{actionLabel(step)}</span>
        {/* Span (not <button>) to avoid an invalid button-in-button: the row
            header itself is a <button>. stopPropagation keeps the path click
            from also toggling the diff. */}
        <span
          role="button"
          tabIndex={0}
          onClick={(e) => { e.stopPropagation(); onOpenFile?.(path) }}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault()
              e.stopPropagation()
              onOpenFile?.(path)
            }
          }}
          className="min-w-0 flex-1 cursor-pointer truncate text-left font-mono text-[0.92em] text-[var(--color-accent)] underline decoration-dotted underline-offset-2 hover:opacity-80"
        >
          {path}
        </span>
        {step.created && (
          <span className="shrink-0 rounded bg-[var(--color-accent)]/15 px-1.5 py-0.5 text-[10px] text-[var(--color-accent)]">
            yeni
          </span>
        )}
        {added > 0 && <span className="shrink-0 text-[var(--color-success)]">+{added}</span>}
        {removed > 0 && <span className="shrink-0 text-[var(--color-danger)]">−{removed}</span>}
        {hasPatch && (
          <span className="ml-1 shrink-0 opacity-50">
            {open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          </span>
        )}
      </button>

      {open && hasPatch && (
        <div className="px-2 pb-2">
          <DiffView text={step.patch || ''} />
        </div>
      )}
    </div>
  )
}
