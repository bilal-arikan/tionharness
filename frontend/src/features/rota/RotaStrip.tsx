// RotaStrip — the one-line "mini rota" in the chat header (brief §8.3): for a
// session that belongs to a coordinator tree, the tree's trajectory as phase
// chips (plan ✓ · kod ● · inceleme ○) plus its status. Click → the Rota screen
// zoomed on that trajectory. Live: the shared lane stream bumps the revision
// and the graph is re-read (useTrajectory).
import type { Session } from '@/types'
import { useTranslation } from 'react-i18next'
import { isInCoordinatorTree } from '@/shared/lib/coordination'
import { STATUS_TONE, trajectoryStatusLabel } from './trajectoryStatus'
import { Badge } from '@/shared/components'
import { phaseGlyph, phaseId, trajectoryProgress } from './trajectoryLayout'
import { rootOf, useTrajectory } from './useTrajectory'

interface Props {
  session: Session
  onOpenTrajectory: (trajectoryId: string) => void
}

export function RotaStrip({ session, onOpenTrajectory }: Props) {
  const { t: translate } = useTranslation('rota')
  const inTree = isInCoordinatorTree(session)
  const root = inTree ? rootOf(session) : ''
  const { trajectory: t } = useTrajectory({ rootSessionId: root || null })
  if (!inTree || !t) return null
  const phases = t.nodes.filter((n) => n.kind === 'phase')
  const progress = trajectoryProgress(t)
  // Which phase this session (a worker) is bound under, if any.
  const mine = t.nodes.find((n) => n.kind === 'session' && n.refId === session.id)?.phaseId
  return (
    <button
      type="button"
      onClick={() => onOpenTrajectory(t.id)}
      className="flex min-w-0 items-center gap-1.5 rounded-lg border border-[var(--color-border)] px-2 py-0.5 text-[11px] text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-text)]"
      title={translate('strip.openTitle', {
        id: t.id,
        template: t.templateRef || translate('common.unplanned'),
        revision: t.revision,
      })}
      data-testid="rota-strip"
    >
      <span className="text-[var(--color-accent)]">◈</span>
      {phases.length === 0 ? (
        <span className="truncate">{translate('strip.noPhases')}</span>
      ) : (
        <span className="flex min-w-0 items-center gap-1 overflow-hidden">
          {phases.map((p, i) => (
            <span key={p.id} className="flex items-center gap-1">
              {i > 0 && <span className="opacity-50">·</span>}
              <span
                className={
                  p.state === 'active'
                    ? 'font-medium text-[var(--color-accent)]'
                    : p.state === 'done'
                      ? 'text-[var(--color-success)]'
                      : p.state === 'failed'
                        ? 'text-[var(--color-danger)]'
                        : ''
                }
                title={translate('strip.phaseTitle', {
                  phase: phaseId(p.id),
                  state: translate(`phase.state.${p.state}`, { defaultValue: p.state }),
                  profile: p.profile ? ` · ${p.profile}` : '',
                  current: mine === p.id ? ` · ${translate('strip.currentSessionHere')}` : '',
                })}
              >
                {p.label || phaseId(p.id)} {phaseGlyph(p.state)}
                {mine === p.id && <span className="ml-0.5 opacity-70">◂</span>}
              </span>
            </span>
          ))}
        </span>
      )}
      <Badge tone={STATUS_TONE[t.status]}>{trajectoryStatusLabel(t.status)}</Badge>
      {progress.total > 0 && (
        <span className="hidden sm:inline">
          {progress.done}/{progress.total}
        </span>
      )}
    </button>
  )
}
