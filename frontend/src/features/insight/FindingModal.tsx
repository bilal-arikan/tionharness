import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, LayoutGrid, Trash2, X } from 'lucide-react'
import type { AppliedEntity, InsightFinding } from '@/types'
import { InfoPopover, ModalOverlay, PaneHeader } from '@/shared/components'
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
  const { t } = useTranslation('insight')
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
            <button
              onClick={onClose}
              className="rounded p-1 hover:bg-[var(--color-surface-2)]"
              aria-label={t('actions.close')}
            >
              <X className="h-4 w-4" />
            </button>
          }
        />

        {/* Body */}
        <div className="flex-1 space-y-3 overflow-auto p-4 text-sm">
          {f.rootCause && (
            <div>
              <div className="mb-0.5 text-xs font-semibold text-[var(--color-text-dim)]">
                {t('finding.rootCause')}
              </div>
              <p className="leading-snug">{f.rootCause}</p>
            </div>
          )}
          {f.proposedFix && (
            <div>
              <div className="mb-0.5 text-xs font-semibold text-[var(--color-text-dim)]">
                {t('finding.proposedFix')}
              </div>
              <p className="leading-snug">{f.proposedFix}</p>
            </div>
          )}
          {f.proposal && (
            <div data-testid="finding-proposal">
              <div className="mb-0.5 flex items-center gap-1 text-xs font-semibold text-[var(--color-text-dim)]">
                {t('finding.recipeProposal')}
                <InfoPopover
                  text={
                    <>
                      {t('finding.recipeHintBefore')} <code>applied</code>{' '}
                      {t('finding.recipeHintAfter', { slug: f.proposal.slug })}
                    </>
                  }
                />
              </div>
              <div className="flex flex-wrap items-center gap-1.5 text-xs">
                <code className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5">
                  {f.proposal.action}
                </code>
                {f.proposal.target && (
                  <span>
                    {t('finding.target')} <code>{f.proposal.target}</code>
                  </span>
                )}
                {f.proposal.value && (
                  <span>
                    {t('finding.value')} <code>{f.proposal.value}</code>
                  </span>
                )}
                {f.proposal.removes && (
                  <span className="text-[var(--color-warning)]">
                    {t('finding.removes')} <code>{f.proposal.removes}</code>
                  </span>
                )}
                <span className="text-[var(--color-text-dim)]">
                  {t('finding.recipe')} {f.proposal.slug}
                  {f.proposal.version ? `@${f.proposal.version}` : ''}
                </span>
              </div>
              <p className="mt-1 text-xs text-[var(--color-text-dim)]">
                {t('finding.evidence')}: {f.proposal.evidence}
              </p>
            </div>
          )}
          {f.filePointer && (
            <div>
              <div className="mb-0.5 text-xs font-semibold text-[var(--color-text-dim)]">
                {t('finding.fileSuggestion')}
              </div>
              <code className="text-xs">{f.filePointer}</code>
            </div>
          )}
          {f.evidenceSessionIds && f.evidenceSessionIds.length > 0 && (
            <div>
              <div className="mb-0.5 text-xs font-semibold text-[var(--color-text-dim)]">
                {t('finding.evidenceSessions')}
              </div>
              <div className="flex flex-wrap gap-1.5">
                {f.evidenceSessionIds.map((sid) => (
                  <button
                    key={sid}
                    onClick={() => onOpenSession(sid)}
                    className="rounded border border-[var(--color-border)] px-1.5 py-0.5 text-xs text-[var(--color-accent)] hover:bg-[var(--color-surface-2)]"
                    title={t('finding.openTranscript')}
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
            {t('finding.applicationEvidence')} (
            <span className="font-normal">{t('finding.applicationEvidenceRequired')}</span>)
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <select
              className={fieldCls}
              value={entityType}
              onChange={(e) => setEntityType(e.target.value)}
              title={t('finding.entityTypeTitle')}
            >
              <option value="">{t('finding.entityTypePlaceholder')}</option>
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
              placeholder={t('finding.entityIdPlaceholder')}
              autoFocus={focusApplied}
            />
          </div>
          {!evidence && (
            <div className="text-[11px] text-[var(--color-text-dim)]">
              {t('finding.missingEvidence')}
            </div>
          )}
        </div>

        {/* Actions */}
        <div className="flex flex-wrap items-center gap-2 border-t border-[var(--color-border)] p-3">
          <button
            onClick={() => act('accepted')}
            className="flex items-center gap-1 rounded-md bg-[var(--color-accent)] px-3 py-1 text-sm text-[var(--color-on-accent)]"
          >
            <Check className="h-3.5 w-3.5" /> {t('actions.accept')}
          </button>
          <button
            onClick={() => evidence && act('applied', evidence)}
            disabled={!evidence}
            title={evidence ? undefined : t('finding.enterEvidenceFirst')}
            className="rounded-md border border-[var(--color-border)] px-3 py-1 text-sm hover:bg-[var(--color-surface-2)] disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:bg-transparent"
          >
            {t('actions.applied')}
          </button>
          <button
            onClick={() => act('verified')}
            className="rounded-md border border-[var(--color-border)] px-3 py-1 text-sm hover:bg-[var(--color-surface-2)]"
          >
            {t('actions.verified')}
          </button>
          <button
            onClick={() => act('dismissed')}
            className="rounded-md border border-[var(--color-border)] px-3 py-1 text-sm text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]"
          >
            {t('actions.dismiss')}
          </button>
          <button
            onClick={() => {
              onAddCard(f)
              onClose()
            }}
            className="flex items-center gap-1 rounded-md border border-[var(--color-border)] px-3 py-1 text-sm hover:bg-[var(--color-surface-2)]"
          >
            <LayoutGrid className="h-3.5 w-3.5" /> {t('actions.addToCard')}
          </button>
          <button
            onClick={() => {
              if (confirm(t('finding.deleteConfirm'))) {
                onDelete(f.id)
                onClose()
              }
            }}
            className="ml-auto flex items-center gap-1 rounded-md px-3 py-1 text-sm text-[var(--color-danger)] hover:bg-[var(--color-danger)]/10"
          >
            <Trash2 className="h-3.5 w-3.5" /> {t('actions.delete')}
          </button>
        </div>
      </div>
    </ModalOverlay>
  )
}
