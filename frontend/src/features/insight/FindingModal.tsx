import { useState } from 'react'
import { Check, LayoutGrid, Trash2, X } from 'lucide-react'
import type { AppliedEntity, InsightFinding } from '@/types'
import { ModalOverlay, PaneHeader } from '@/shared/components'
import { ChannelBadge, SeverityBadge, RegressedBadge, StatusBadge } from './insightBadges'

interface Props {
  f: InsightFinding
  onClose: () => void
  onStatus: (id: string, status: string, evidence?: AppliedEntity) => void
  onDelete: (id: string) => void
  onAddCard: (f: InsightFinding) => void
  onOpenSession: (sid: string) => void
  /** Open with the "applied" evidence form focused (e.g. after a drag onto that column). */
  focusApplied?: boolean
}

// Workspace entity kinds a fix can land on. Free-form on the wire; this list is
// the guided set so evidence stays comparable across findings.
const ENTITY_TYPES = [
  'skill',
  'agent',
  'hook',
  'automation',
  'flow',
  'schedule',
  'mcp-server',
  'task',
  'other',
]

const fieldCls =
  'rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm'

// FindingModal is the click-through detail popup for one finding: full content
// (root cause / proposed fix / file / evidence) plus the lifecycle + delete
// actions. Mirrors the board's TaskFormModal role for the Insight kanban.
export function FindingModal({
  f,
  onClose,
  onStatus,
  onDelete,
  onAddCard,
  onOpenSession,
  focusApplied,
}: Props) {
  const [entityType, setEntityType] = useState(f.appliedEntity?.entityType ?? '')
  const [entityId, setEntityId] = useState(f.appliedEntity?.entityId ?? '')
  const evidence: AppliedEntity | undefined =
    entityType.trim() && entityId.trim()
      ? { entityType: entityType.trim(), entityId: entityId.trim() }
      : undefined

  const act = (status: string, ev?: AppliedEntity) => {
    onStatus(f.id, status, ev)
    onClose()
  }
  return (
    <ModalOverlay onClose={onClose}>
      <div className="flex max-h-[86vh] w-[min(680px,94vw)] flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-[var(--shadow-lg)]">
        {/* Header */}
        <PaneHeader
          title={f.title}
          secondary={
            <>
              <ChannelBadge channel={f.channel} />
              {f.severity && <SeverityBadge severity={f.severity} />}
              {f.regressed && <RegressedBadge />}
              {f.status && f.status !== 'new' && <StatusBadge status={f.status} />}
              {f.occurrences > 1 && (
                <span className="text-xs text-[var(--color-text-dim)]">×{f.occurrences}</span>
              )}
              <span className="text-xs text-[var(--color-text-dim)]">{f.lensId}</span>
            </>
          }
          right={
            <button onClick={onClose} className="rounded p-1 hover:bg-[var(--color-surface-2)]">
              <X className="h-4 w-4" />
            </button>
          }
        />

        {/* Body */}
        <div className="flex-1 space-y-3 overflow-auto p-4 text-sm">
          {f.rootCause && (
            <div>
              <div className="mb-0.5 text-xs font-semibold text-[var(--color-text-dim)]">
                Kök neden
              </div>
              <p className="leading-snug">{f.rootCause}</p>
            </div>
          )}
          {f.proposedFix && (
            <div>
              <div className="mb-0.5 text-xs font-semibold text-[var(--color-text-dim)]">
                Önerilen çözüm
              </div>
              <p className="leading-snug">{f.proposedFix}</p>
            </div>
          )}
          {f.filePointer && (
            <div>
              <div className="mb-0.5 text-xs font-semibold text-[var(--color-text-dim)]">
                Dosya (öneri)
              </div>
              <code className="text-xs">{f.filePointer}</code>
            </div>
          )}
          {f.evidenceSessionIds && f.evidenceSessionIds.length > 0 && (
            <div>
              <div className="mb-0.5 text-xs font-semibold text-[var(--color-text-dim)]">
                Kanıt oturumları
              </div>
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
          <div className="pt-1 text-[11px] text-[var(--color-text-dim)]">
            <code className="opacity-70">{f.sig}</code>
          </div>
        </div>

        {/* Evidence for "applied" — mandatory, so it is collected before the action */}
        <div
          className={`space-y-1.5 border-t border-[var(--color-border)] px-3 pb-2 pt-2 ${
            focusApplied ? 'bg-[var(--color-accent-soft)]' : ''
          }`}
        >
          <div className="text-xs font-semibold text-[var(--color-text-dim)]">
            Uygulama kanıtı (
            <span className="font-normal">
              &quot;Uygulandı&quot; için zorunlu — otomatik doğrulama buna bakar
            </span>
            )
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <select
              className={fieldCls}
              value={entityType}
              onChange={(e) => setEntityType(e.target.value)}
              title="Değiştirdiğin varlık türü"
            >
              <option value="">Varlık türü…</option>
              {ENTITY_TYPES.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
            <input
              type="text"
              className={`${fieldCls} min-w-52 flex-1`}
              value={entityId}
              onChange={(e) => setEntityId(e.target.value)}
              placeholder="Varlık id'si (ör. tionharness-tool-discovery)"
              autoFocus={focusApplied}
            />
          </div>
          {!evidence && (
            <div className="text-[11px] text-[var(--color-text-dim)]">
              Varlık türü ve id'si dolmadan &quot;Uygulandı&quot; işaretlenemez: kanıtsız
              &quot;uygulandı&quot; doğrulanamaz bir iddiadır.
            </div>
          )}
        </div>

        {/* Actions */}
        <div className="flex flex-wrap items-center gap-2 border-t border-[var(--color-border)] p-3">
          <button
            onClick={() => act('accepted')}
            className="flex items-center gap-1 rounded-md bg-[var(--color-accent)] px-3 py-1 text-sm text-[var(--color-on-accent)]"
          >
            <Check className="h-3.5 w-3.5" /> Kabul
          </button>
          <button
            onClick={() => evidence && act('applied', evidence)}
            disabled={!evidence}
            title={evidence ? undefined : 'Önce uygulama kanıtını (varlık türü + id) gir'}
            className="rounded-md border border-[var(--color-border)] px-3 py-1 text-sm hover:bg-[var(--color-surface-2)] disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:bg-transparent"
          >
            Uygulandı
          </button>
          <button
            onClick={() => act('verified')}
            className="rounded-md border border-[var(--color-border)] px-3 py-1 text-sm hover:bg-[var(--color-surface-2)]"
          >
            Doğrulandı
          </button>
          <button
            onClick={() => act('dismissed')}
            className="rounded-md border border-[var(--color-border)] px-3 py-1 text-sm text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]"
          >
            Yoksay
          </button>
          <button
            onClick={() => {
              onAddCard(f)
              onClose()
            }}
            className="flex items-center gap-1 rounded-md border border-[var(--color-border)] px-3 py-1 text-sm hover:bg-[var(--color-surface-2)]"
          >
            <LayoutGrid className="h-3.5 w-3.5" /> Karta ekle
          </button>
          <button
            onClick={() => {
              if (confirm('Bu bulgu silinsin mi?')) {
                onDelete(f.id)
                onClose()
              }
            }}
            className="ml-auto flex items-center gap-1 rounded-md px-3 py-1 text-sm text-[var(--color-danger)] hover:bg-[var(--color-danger)]/10"
          >
            <Trash2 className="h-3.5 w-3.5" /> Sil
          </button>
        </div>
      </div>
    </ModalOverlay>
  )
}
