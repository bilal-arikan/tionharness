import type { LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { Session } from '@/types'
import { relativeTime } from '@/shared/lib/time'
import { modelDisplayName } from '@/shared/lib/modelLabel'
import { RunStateBadge, StatusPill } from './sessionKindBadges'

interface Props {
  session: Session
  kindLabel: string
  kindIcon: LucideIcon
  // Model shown for the row (see sessionListModel); empty when unknown.
  displayedModel?: string
  isStreaming: boolean
  // Outcome of the last finished task/flow run, when there is one.
  lastStatus?: string
}

// SessionRowMeta is the single grey line under a session row's title: kind,
// model, recency, message count and how the session was created, as plain text.
// The run outcome sits at the right end; the session ID is revealed there only
// while the row is hovered or focused (the row's tooltip always carries it).
export function SessionRowMeta({
  session: s,
  kindLabel,
  kindIcon: KindIcon,
  displayedModel,
  isStreaming,
  lastStatus,
}: Props) {
  const { t } = useTranslation('sessions')
  const origin =
    s.kind === 'spawned'
      ? {
          label: s.parentSessionId ? t('sidebar.handoff') : t('sidebar.spawn'),
          title: s.parentSessionId ? t('sidebar.createdByHandoff') : t('sidebar.createdBySpawn'),
        }
      : null
  return (
    <span
      data-testid="session-row-meta"
      className="flex items-center gap-1.5 text-[11px] text-[var(--color-text-dim)]"
    >
      <KindIcon size={11} className="shrink-0 opacity-70" />
      <span className="min-w-0 flex-1 truncate">
        {kindLabel}
        {displayedModel && (
          <span title={t('sidebar.model', { model: displayedModel })}>
            {' · '}
            {modelDisplayName(displayedModel)}
          </span>
        )}
        {' · '}
        {isStreaming ? (
          <span className="font-medium text-[var(--color-success)]">{t('sidebar.writing')}</span>
        ) : (
          <>
            {relativeTime(s.updatedAt)}
            {' · '}
            {t('sidebar.messages', { count: s.messageCount })}
          </>
        )}
        {origin && (
          <span data-testid="session-row-origin" title={origin.title}>
            {' · '}
            {origin.label}
          </span>
        )}
      </span>
      {/* Finished task/flow runs carry a pass/fail pill. */}
      {!isStreaming && lastStatus && <StatusPill status={lastStatus} />}
      {/* How the last BACKGROUND turn ended (spawn/schedule runs). Independent
          of the archive filter: this is the run outcome, `state` is visibility.
          Hidden while a turn is live — the pulsing dot and "yazıyor…" already
          say what is happening now, and a stale outcome next to them reads as
          contradictory. */}
      {!isStreaming && <RunStateBadge runState={s.runState} />}
      <span
        data-testid="session-row-id"
        className="hidden shrink-0 font-mono group-focus-within:inline group-hover:inline"
        title={t('sidebar.sessionId')}
      >
        {s.id}
      </span>
    </span>
  )
}
