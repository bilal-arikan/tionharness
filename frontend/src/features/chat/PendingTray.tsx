// A staged intervention waiting above the composer while a turn streams:
// a queued message (sent when the turn ends), a steer (live guidance sent after a
// short cancellable delay), or the message the server just dispatched
// ('dispatching' — no longer cancellable). Queue + steer can be removed before
// they are applied.
//
// 'dispatching' is kept in the MODEL but is not rendered: the tray is the list of
// things still waiting and still cancellable, and a dispatched message is neither.
// It becomes the user's own bubble in the transcript, so a row here only duplicated
// it. The kind still drives hub reconciliation (chatStreamHub drops it when the
// bubble lands) and gates the 'holding' row, so it must not be removed from the
// data model — only hidden. See hiddenInTray below.
//
// 'failed' is the dispatched head whose turn died in PREFLIGHT (session_not_found,
// agent_not_found, persist_error): the server never published a user_message, so
// the text exists nowhere else — not in the transcript, not in the queue. Hiding it
// like 'dispatching' would lose the user's words silently, so it is rendered, with
// its text intact and removable once the user has read it.
export interface PendingItem {
  id: string
  text: string
  kind: 'queue' | 'steer' | 'dispatching' | 'holding' | 'failed'
  // The session this intervention belongs to. The tray is filtered to the
  // active session, and queue flush / steer dispatch target this session's
  // turn — so staged items for a background turn never apply to another.
  sid: string
}

interface Props {
  items: PendingItem[]
  onRemove: (id: string) => void
  // Promote a waiting message to dispatch next ("öne al"). Optional.
  onSendNext?: (id: string) => void
  // Convert a waiting message into live guidance for the turn already running
  // ("şimdi yönlendir"). Optional.
  onSteerNow?: (id: string) => void
  // Whether a steer can actually land: a turn is streaming AND the server reports
  // it can carry mid-turn guidance (queue_update.steerable). The action is shown
  // disabled (with the reason) rather than hidden — the button must not appear and
  // vanish as turns come and go.
  canSteer?: boolean
  // Distinguishes an idle session from a running turn without a delivery channel.
  turnRunning?: boolean
  // Clear the whole waiting queue. Optional; shown when 2+ queue items wait.
  onClear?: () => void
}

import { AlertTriangle, ArrowUp, CornerDownRight, Hourglass, Loader, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ComposerCard } from './ComposerCard'

// Kinds the tray does not render. A dispatched message is already on its way to the
// transcript as a user bubble; showing it here too was a duplicate the user cannot
// act on.
function hiddenInTray(it: PendingItem): boolean {
  return it.kind === 'dispatching'
}

// PendingTray lists the session's WAITING backend queue (+ any steers) above the
// composer. Queue items show their position (#N), can be promoted to run next,
// removed individually, or cleared all at once.
export function PendingTray({
  items,
  onRemove,
  onSendNext,
  onSteerNow,
  canSteer = false,
  turnRunning = false,
  onClear,
}: Props) {
  const { t } = useTranslation('chatStatus')
  // Filter BEFORE the empty check: a tray whose only item is the dispatched head
  // must render nothing at all, not an empty card with the "Bekleyenler" header.
  const visible = items.filter((it) => !hiddenInTray(it))
  if (visible.length === 0) return null
  const queueCount = visible.filter((it) => it.kind === 'queue').length
  let qIndex = 0
  return (
    <ComposerCard tone="muted" className="flex flex-col gap-1.5 px-3 py-2">
      <div className="flex items-center justify-between">
        <span className="text-[10px] font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
          {visible.some((it) => it.kind === 'failed')
            ? t('pending.headerFailed')
            : t('pending.header')}
        </span>
        {onClear && queueCount > 1 && (
          <button
            onClick={onClear}
            className="text-[10px] font-medium text-[var(--color-text-dim)] transition hover:text-[var(--color-danger)]"
          >
            {t('pending.clearQueue', { count: queueCount })}
          </button>
        )}
      </div>
      {visible.map((it) => {
        const pos = it.kind === 'queue' ? ++qIndex : 0
        if (it.kind === 'holding') {
          // What the session is busy with right now (an autonomous turn from the
          // admission queue). Informational: it tells the user WHAT their message is
          // waiting behind, so a queued message never looks stuck for no reason.
          return (
            <div
              key={it.id}
              className="flex items-center gap-2 rounded-lg border border-dashed border-[var(--color-border)] px-2.5 py-1.5 text-sm"
            >
              <span
                className="inline-flex shrink-0 items-center gap-1 rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[10px] font-semibold text-[var(--color-text-dim)]"
                title={t('pending.currentTitle')}
              >
                <Loader size={11} />
                {t('pending.current')}
              </span>
              <span className="min-w-0 flex-1 truncate text-[var(--color-text-dim)]">
                {it.text}
              </span>
            </div>
          )
        }
        if (it.kind === 'failed') {
          // The turn never started, so this text is the ONLY copy of what the user
          // typed. Shown in the danger tone with the reason, and removable — the
          // user can copy it out or dismiss it, but it never disappears on its own.
          return (
            <div
              key={it.id}
              className="flex items-center gap-2 rounded-lg border border-[color-mix(in_srgb,var(--color-danger)_40%,transparent)] bg-[color-mix(in_srgb,var(--color-danger)_8%,transparent)] px-2.5 py-1.5 text-sm"
              data-testid={`pending-failed-${it.id}`}
            >
              <span
                className="inline-flex shrink-0 items-center gap-1 rounded bg-[color-mix(in_srgb,var(--color-danger)_20%,transparent)] px-1.5 py-0.5 text-[10px] font-semibold text-[var(--color-danger)]"
                title={t('pending.failedTitle')}
              >
                <AlertTriangle size={11} />
                {t('pending.failed')}
              </span>
              <span className="min-w-0 flex-1 truncate text-[var(--color-text)]">{it.text}</span>
              <button
                onClick={() => onRemove(it.id)}
                title={t('pending.dismiss')}
                data-testid={`pending-remove-${it.id}`}
                className="shrink-0 rounded p-0.5 text-[var(--color-text-dim)] transition hover:text-[var(--color-danger)]"
              >
                <X size={13} />
              </button>
            </div>
          )
        }
        return (
          <div
            key={it.id}
            className="flex items-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2.5 py-1.5 text-sm"
          >
            <span
              className={`inline-flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-semibold ${
                it.kind === 'steer'
                  ? 'bg-[color-mix(in_srgb,var(--color-warning)_20%,transparent)] text-[var(--color-warning)]'
                  : 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
              }`}
              title={it.kind === 'steer' ? t('pending.steerTitle') : t('pending.queueTitle')}
            >
              {it.kind === 'steer' ? <CornerDownRight size={11} /> : <Hourglass size={11} />}
              {it.kind === 'steer' ? t('pending.steer') : t('pending.position', { position: pos })}
            </span>
            <span className="min-w-0 flex-1 truncate text-[var(--color-text)]">{it.text}</span>
            {it.kind === 'queue' && onSteerNow && (
              <button
                onClick={() => onSteerNow(it.id)}
                disabled={!canSteer}
                title={
                  canSteer
                    ? t('pending.steerCurrent')
                    : turnRunning
                      ? t('pending.steerUnavailable')
                      : t('pending.steerNoTurn')
                }
                data-testid={`pending-steer-${it.id}`}
                className="shrink-0 rounded p-0.5 text-[var(--color-text-dim)] transition hover:text-[var(--color-warning)] disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:text-[var(--color-text-dim)]"
              >
                <CornerDownRight size={14} />
              </button>
            )}
            {it.kind === 'queue' && onSendNext && pos > 1 && (
              <button
                onClick={() => onSendNext(it.id)}
                title={t('pending.promote')}
                className="shrink-0 rounded p-0.5 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
              >
                <ArrowUp size={14} />
              </button>
            )}
            <button
              onClick={() => onRemove(it.id)}
              title={t('pending.remove')}
              className="shrink-0 rounded p-0.5 text-[var(--color-text-dim)] transition hover:text-[var(--color-danger)]"
            >
              <X size={14} />
            </button>
          </div>
        )
      })}
    </ComposerCard>
  )
}
