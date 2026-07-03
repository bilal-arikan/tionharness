import { useState } from 'react'
import { Pencil, ChevronDown, ChevronRight } from 'lucide-react'
import type { TurnStep } from '../../types'
import { DiffView } from '../markdown/DiffView'
import { synthDiffData } from '../../lib/diff'
import { toolBase } from '../../lib/tools'
import { shortPath } from '../../lib/paths'

interface Props {
  step: TurnStep
  onOpenFile?: (path: string) => void
}

// A friendly verb for the mutating tool that produced this diff. The tool name is
// lower-cased first so it matches whether the step reports "Edit"/"Write" (the
// shared, claude-cli-style names) or a namespaced/legacy variant.
function actionLabel(step: TurnStep, created: boolean): string {
  if (created) return 'Oluştur'
  const base = (step.tool || '').toLowerCase()
  if (base === 'edit' || base === 'edit_file' || base === 'multiedit') return 'Düzenle'
  if (base === 'write' || base === 'write_file') return 'Yaz'
  return 'Değişiklik'
}

// DiffCard renders a single file mutation (Write / Edit) as a compact
// row: ✏️ icon, action label, clickable path and the +added/−removed line
// counts — expandable to the full unified patch. Mirrors the file-change cards
// in External Agent / Claude Code chat.
//
// It serves two paths: native tool-loop edits arrive as a `diff` step carrying a
// precomputed patch/added/removed/path; claude-cli edits arrive as a `tool` step
// (the CLI applied the change itself, so no FileDiff was recorded) — for those we
// synthesize the patch and line counts from the tool input, so both render the
// same panel.
export function DiffCard({ step, onOpenFile }: Props) {
  const [open, setOpen] = useState(false)
  const synth = step.patch?.trim() ? null : synthDiffData(toolBase(step.tool || ''), step.input)
  const path = step.path || synth?.path || ''
  const patch = step.patch?.trim() ? step.patch : synth?.patch || ''
  const added = step.added || synth?.added || 0
  const removed = step.removed || synth?.removed || 0
  const created = !!step.created
  const hasPatch = !!patch.trim()

  return (
    <div className="overflow-hidden rounded-lg bg-[var(--color-surface)]">
      <button
        onClick={() => setOpen((o) => hasPatch ? !o : o)}
        className={`flex w-full items-center gap-2 rounded-lg px-3 py-1.5 text-left text-xs ${
          hasPatch ? 'hover:bg-[var(--color-surface-2)]' : 'cursor-default'
        }`}
      >
        <Pencil size={14} className="shrink-0 text-[var(--color-text-dim)]" />
        <span className="shrink-0 font-medium text-[var(--color-text)]">{actionLabel(step, created)}</span>
        {/* The path is shown short (…/dir/file) so it no longer spans the whole
            row; only the text itself opens the file (span, not <button>, to keep
            valid HTML inside the header <button>; stopPropagation so it doesn't
            also toggle). The flex-1 remainder stays part of the header button, so
            clicking the empty area (or the chevron) folds the diff. */}
        <span className="flex min-w-0 flex-1 items-center">
          <span
            role="button"
            tabIndex={0}
            title={path}
            onClick={(e) => { e.stopPropagation(); onOpenFile?.(path) }}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                e.stopPropagation()
                onOpenFile?.(path)
              }
            }}
            className="max-w-full cursor-pointer truncate text-left font-mono text-[0.92em] text-[var(--color-accent)] underline decoration-dotted underline-offset-2 hover:opacity-80"
          >
            {shortPath(path)}
          </span>
        </span>
        {created && (
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
          <DiffView text={patch} />
        </div>
      )}
    </div>
  )
}
