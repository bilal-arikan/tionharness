// SavedViewMenu — the view picker at the left of the filter bar. Lists the
// built-in presets, then the workspace's saved views (each with rename/delete),
// and carries the "unsaved changes" indicator.

import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, Pencil, Trash2 } from 'lucide-react'
import { useOutsideClick } from '@/shared/hooks/useOutsideClick'
import type { BoardViewDef } from '@/types'
import { isBuiltinId } from './boardViewTypes'

interface Props {
  allViews: BoardViewDef[]
  selectedId: string
  dirty: boolean
  onSelect: (id: string) => void
  onRename: (id: string, label: string) => Promise<void>
  onDelete: (id: string) => Promise<void>
}

export function SavedViewMenu({
  allViews,
  selectedId,
  dirty,
  onSelect,
  onRename,
  onDelete,
}: Props) {
  const { t } = useTranslation('tasks')
  const [open, setOpen] = useState(false)
  const ref = useOutsideClick<HTMLDivElement>(() => setOpen(false), open)
  const builtins = allViews.filter((v) => isBuiltinId(v.id))
  const selected = allViews.find((v) => v.id === selectedId) ?? builtins[0]
  const saved = allViews.filter((v) => !isBuiltinId(v.id))

  const rename = async (v: BoardViewDef) => {
    const label = prompt(t('views.namePrompt'), v.label)
    if (label === null) return
    await onRename(v.id, label)
  }

  const remove = async (v: BoardViewDef) => {
    if (!confirm(t('views.deleteConfirm', { label: v.label }))) return
    await onDelete(v.id)
    setOpen(false)
  }

  const row = (v: BoardViewDef) => (
    <div
      key={v.id}
      className={`group flex items-center rounded transition hover:bg-[var(--color-surface-2)] ${
        v.id === selectedId ? 'text-[var(--color-accent)]' : 'text-[var(--color-text)]'
      }`}
    >
      <button
        onClick={() => {
          onSelect(v.id)
          setOpen(false)
        }}
        className="flex min-w-0 flex-1 items-center gap-2 px-2 py-1.5 text-left text-xs"
      >
        <span className="w-4 flex-shrink-0 text-center">{v.icon ?? '▤'}</span>
        <span className="truncate">{v.label}</span>
      </button>
      {!isBuiltinId(v.id) && (
        <span className="flex flex-shrink-0 items-center gap-0.5 pr-1 opacity-0 transition group-hover:opacity-100">
          <button
            onClick={() => void rename(v)}
            title={t('actions.rename')}
            className="rounded p-1 text-[var(--color-text-dim)] hover:text-[var(--color-accent)]"
          >
            <Pencil size={11} />
          </button>
          <button
            onClick={() => void remove(v)}
            title={t('actions.delete')}
            className="rounded p-1 text-[var(--color-text-dim)] hover:text-[var(--color-danger)]"
          >
            <Trash2 size={11} />
          </button>
        </span>
      )}
    </div>
  )

  return (
    <div ref={ref} className="relative">
      <button
        data-testid="board-view-menu"
        onClick={() => setOpen((v) => !v)}
        className="flex items-center gap-1 whitespace-nowrap rounded border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-1 text-xs text-[var(--color-text)] transition hover:border-[var(--color-accent)]"
      >
        <span>{selected.icon ?? '▤'}</span>
        <span className="max-w-[140px] truncate">{selected.label}</span>
        {dirty && (
          <span
            title={t('views.unsavedChanges')}
            className="h-1.5 w-1.5 rounded-full bg-[var(--color-accent)]"
          />
        )}
        <ChevronDown size={12} />
      </button>
      {open && (
        <div className="absolute left-0 z-30 mt-1 w-60 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-[var(--shadow-md)]">
          <div className="px-2 py-1 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
            {t('views.builtinSection')}
          </div>
          {builtins.map(row)}
          <div className="mt-1 border-t border-[var(--color-border)] px-2 pb-1 pt-2 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
            {t('views.savedSection')}
          </div>
          {saved.length === 0 ? (
            <div className="px-2 pb-2 text-xs text-[var(--color-text-dim)]">
              {t('views.emptySaved')}
            </div>
          ) : (
            saved.map(row)
          )}
        </div>
      )}
    </div>
  )
}
