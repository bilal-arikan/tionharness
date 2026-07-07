import { Loader2, Check, Pencil, X } from 'lucide-react'
import type { SessionInfo } from '@/types'
import { Pill } from './SessionDetailBits'

interface Props {
  info: SessionInfo
  editingTitle: boolean
  titleDraft: string
  savingTitle: boolean
  setTitleDraft: (v: string) => void
  setEditingTitle: (v: boolean) => void
  startEditTitle: () => void
  commitTitle: () => void
  onSelectSession?: (id: string) => void
}

// Title + status pills + context-reset lineage for the session inspector.
export function SessionTitleBlock({
  info,
  editingTitle,
  titleDraft,
  savingTitle,
  setTitleDraft,
  setEditingTitle,
  startEditTitle,
  commitTitle,
  onSelectSession,
}: Props) {
  return (
    <div>
      {editingTitle ? (
        <div className="flex items-center gap-1.5">
          <input
            autoFocus
            value={titleDraft}
            onChange={(e) => setTitleDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') commitTitle()
              else if (e.key === 'Escape') setEditingTitle(false)
            }}
            disabled={savingTitle}
            placeholder="Sohbet başlığı"
            className="min-w-0 flex-1 rounded border border-[var(--color-accent)] bg-[var(--color-bg)] px-2 py-1 text-sm font-semibold text-[var(--color-text)] outline-none disabled:opacity-50"
          />
          <button
            onClick={commitTitle}
            disabled={savingTitle}
            title="Kaydet"
            className="rounded p-1 text-[var(--color-accent)] transition hover:opacity-80 disabled:opacity-40"
          >
            {savingTitle ? <Loader2 size={15} className="animate-spin" /> : <Check size={15} />}
          </button>
          <button
            onClick={() => setEditingTitle(false)}
            disabled={savingTitle}
            title="İptal"
            className="rounded p-1 text-[var(--color-text-dim)] transition hover:text-[var(--color-text)] disabled:opacity-40"
          >
            <X size={15} />
          </button>
        </div>
      ) : (
        <div className="group flex items-center gap-1.5">
          <h3 className="min-w-0 flex-1 truncate text-sm font-semibold text-[var(--color-text)]" title={info.title}>
            {info.title || 'Yeni sohbet'}
          </h3>
          <button
            onClick={startEditTitle}
            title="Başlığı düzenle"
            className="shrink-0 rounded p-1 text-[var(--color-text-dim)] opacity-0 transition hover:text-[var(--color-accent)] group-hover:opacity-100"
          >
            <Pencil size={13} />
          </button>
        </div>
      )}
      <div className="mt-1 flex flex-wrap items-center gap-1.5">
        {info.state && <Pill>{info.state}</Pill>}
        {info.kind && <Pill>{info.kind}</Pill>}
        {info.unread && <Pill accent>okunmadı</Pill>}
      </div>
      {/* Context-reset lineage: this session continues an earlier one. */}
      {info.parentSessionId && (
        <button
          type="button"
          onClick={() => onSelectSession?.(info.parentSessionId!)}
          disabled={!onSelectSession}
          className="mt-1.5 inline-flex items-center gap-1 rounded-md border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-1 text-xs text-[var(--color-text-dim)] hover:text-[var(--color-text)] disabled:cursor-default disabled:hover:text-[var(--color-text-dim)]"
          title="Bu oturum bir context reset (handoff) ile önceki oturumdan devraldı"
        >
          ↩ Devraldığı oturum: <span className="font-mono">{info.parentSessionId}</span>
        </button>
      )}
    </div>
  )
}
