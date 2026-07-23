import { Check, LayoutGrid, Trash2, X } from 'lucide-react'
import type { InsightFinding } from '@/types'
import { ModalOverlay } from '@/shared/components'
import { ChannelBadge, SeverityBadge, RegressedBadge, StatusBadge } from './insightBadges'

interface Props {
  f: InsightFinding
  onClose: () => void
  onStatus: (id: string, status: string) => void
  onDelete: (id: string) => void
  onAddCard: (f: InsightFinding) => void
  onOpenSession: (sid: string) => void
}

// FindingModal is the click-through detail popup for one finding: full content
// (root cause / proposed fix / file / evidence) plus the lifecycle + delete
// actions. Mirrors the board's TaskFormModal role for the Insight kanban.
export function FindingModal({ f, onClose, onStatus, onDelete, onAddCard, onOpenSession }: Props) {
  const act = (status: string) => {
    onStatus(f.id, status)
    onClose()
  }
  return (
    <ModalOverlay onClose={onClose}>
      <div className="flex max-h-[86vh] w-[min(680px,94vw)] flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-2xl">
        {/* Header */}
        <div className="flex items-start gap-2 border-b border-[var(--color-border)] p-4">
          <div className="min-w-0 flex-1">
            <div className="mb-1.5 flex flex-wrap items-center gap-1.5">
              <ChannelBadge channel={f.channel} />
              {f.severity && <SeverityBadge severity={f.severity} />}
              {f.regressed && <RegressedBadge />}
              {f.status && f.status !== 'new' && <StatusBadge status={f.status} />}
              {f.occurrences > 1 && (
                <span className="text-xs text-[var(--color-text-muted)]">×{f.occurrences}</span>
              )}
              <span className="text-xs text-[var(--color-text-muted)]">{f.lensId}</span>
            </div>
            <h2 className="text-base font-semibold leading-snug">{f.title}</h2>
          </div>
          <button onClick={onClose} className="rounded p-1 hover:bg-[var(--color-surface-2)]">
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 space-y-3 overflow-auto p-4 text-sm">
          {f.rootCause && (
            <div>
              <div className="mb-0.5 text-xs font-semibold text-[var(--color-text-muted)]">Kök neden</div>
              <p className="leading-snug">{f.rootCause}</p>
            </div>
          )}
          {f.proposedFix && (
            <div>
              <div className="mb-0.5 text-xs font-semibold text-[var(--color-text-muted)]">Önerilen çözüm</div>
              <p className="leading-snug">{f.proposedFix}</p>
            </div>
          )}
          {f.filePointer && (
            <div>
              <div className="mb-0.5 text-xs font-semibold text-[var(--color-text-muted)]">Dosya (öneri)</div>
              <code className="text-xs">{f.filePointer}</code>
            </div>
          )}
          {f.evidenceSessionIds && f.evidenceSessionIds.length > 0 && (
            <div>
              <div className="mb-0.5 text-xs font-semibold text-[var(--color-text-muted)]">Kanıt oturumları</div>
              <div className="flex flex-wrap gap-1.5">
                {f.evidenceSessionIds.map((sid) => (
                  <button
                    key={sid}
                    onClick={() => onOpenSession(sid)}
                    className="rounded border border-[var(--color-border)] px-1.5 py-0.5 text-xs text-[var(--color-accent)] hover:bg-[var(--color-surface-2)]"
                    title="Oturum transkriptine git"
                  >
                    {sid}
                  </button>
                ))}
              </div>
            </div>
          )}
          <div className="pt-1 text-[11px] text-[var(--color-text-muted)]">
            <code className="opacity-70">{f.sig}</code>
          </div>
        </div>

        {/* Actions */}
        <div className="flex flex-wrap items-center gap-2 border-t border-[var(--color-border)] p-3">
          <button onClick={() => act('accepted')} className="flex items-center gap-1 rounded-md bg-[var(--color-accent)] px-3 py-1 text-sm text-white">
            <Check className="h-3.5 w-3.5" /> Kabul
          </button>
          <button onClick={() => act('applied')} className="rounded-md border border-[var(--color-border)] px-3 py-1 text-sm hover:bg-[var(--color-surface-2)]">Uygulandı</button>
          <button onClick={() => act('verified')} className="rounded-md border border-[var(--color-border)] px-3 py-1 text-sm hover:bg-[var(--color-surface-2)]">Doğrulandı</button>
          <button onClick={() => act('dismissed')} className="rounded-md border border-[var(--color-border)] px-3 py-1 text-sm text-[var(--color-text-muted)] hover:bg-[var(--color-surface-2)]">Yoksay</button>
          <button onClick={() => { onAddCard(f); onClose() }} className="flex items-center gap-1 rounded-md border border-[var(--color-border)] px-3 py-1 text-sm hover:bg-[var(--color-surface-2)]">
            <LayoutGrid className="h-3.5 w-3.5" /> Karta ekle
          </button>
          <button
            onClick={() => { if (confirm('Bu bulgu silinsin mi?')) { onDelete(f.id); onClose() } }}
            className="ml-auto flex items-center gap-1 rounded-md px-3 py-1 text-sm text-[var(--color-danger)] hover:bg-[var(--color-danger)]/10"
          >
            <Trash2 className="h-3.5 w-3.5" /> Sil
          </button>
        </div>
      </div>
    </ModalOverlay>
  )
}
