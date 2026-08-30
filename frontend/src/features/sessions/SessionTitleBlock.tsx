import { Loader2, Check, Pencil, X, Sparkles } from 'lucide-react'
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
  // AI title generation, surfaced right next to the manual edit control.
  onGenerateTitle: () => void
  titling: boolean
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
  onGenerateTitle,
  titling,
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
        // The two title controls stay VISIBLE at rest (no hover gating): they were
        // `opacity-0 group-hover:opacity-100` ghosts, which made renaming and AI
        // title generation undiscoverable unless you happened to hover the row.
        <div className="flex items-center gap-1.5">
          <h3
            className="min-w-0 flex-1 truncate text-sm font-semibold text-[var(--color-text)]"
            title={info.title}
          >
            {info.title || 'Yeni sohbet'}
          </h3>
          <button
            onClick={onGenerateTitle}
            disabled={info.messageCount === 0 || titling}
            title={titling ? 'Başlık üretiliyor…' : 'AI ile başlık üret'}
            aria-label="AI ile başlık üret"
            className="shrink-0 rounded p-1 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)] disabled:cursor-default disabled:opacity-30"
          >
            {titling ? <Loader2 size={13} className="animate-spin" /> : <Sparkles size={13} />}
          </button>
          <button
            onClick={startEditTitle}
            title="Başlığı düzenle"
            aria-label="Başlığı düzenle"
            className="shrink-0 rounded p-1 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
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
          title={
            info.executionType === 'subagent'
              ? 'Bu alt-ajan oturumunu başlatan parent oturum'
              : 'Bu oturum bir context reset (handoff) ile önceki oturumdan devraldı'
          }
        >
          ↩ {info.executionType === 'subagent' ? 'Parent oturum' : 'Devraldığı oturum'}:{' '}
          <span className="font-mono">{info.parentSessionId}</span>
        </button>
      )}
    </div>
  )
}
