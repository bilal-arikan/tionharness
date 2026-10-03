import { i18next } from '@/i18n'
// diffSignals — the pure half of useWorkspaceSignals: compares two lane-store
// snapshots and returns the toasts worth showing. Store-free so it runs in the
// node test environment.
import type { LaneState } from '@/shared/lib/laneModel'
import { SKIP_REASON_LABEL } from '@/features/schedules/fireMeta'

const TRAJ_STATUS_TEXT: Record<string, string> = {
  done: 'shell.trajectoryDone',
  failed: 'shell.trajectoryFailed',
  abandoned: 'shell.trajectoryAbandoned',
  waiting: 'shell.trajectoryWaiting',
}

// SignalLevel maps 1:1 onto a toast tone (see useWorkspaceSignals).
export type SignalLevel = 'info' | 'success' | 'warning' | 'error'

// diffSignals compares two lane snapshots and returns the toasts to show —
// pure, so it is unit-testable without the store.
export function diffSignals(
  prev: LaneState,
  next: LaneState,
  enabled: (type: string) => boolean = () => true,
): { level: SignalLevel; text: string }[] {
  const out: { level: SignalLevel; text: string }[] = []
  if (enabled('automation')) {
    const lastSeq = prev.fires.length ? prev.fires[prev.fires.length - 1].seq : 0
    for (const f of next.fires) {
      if (f.seq <= lastSeq) continue
      const name = f.name || f.automationId
      if (f.outcome === 'fired') {
        out.push({
          level: 'info',
          text: i18next.t('shell.automationFired', {
            name,
            session: f.sessionId ? ' → ' + f.sessionId : '',
          }),
        })
      } else if (f.reason && f.reason !== 'cooldown') {
        // Cooldown skips are routine noise; the others say a rule is stuck.
        out.push({
          level: 'warning',
          text: i18next.t('shell.automationSkipped', {
            name,
            reason: SKIP_REASON_LABEL[f.reason] ?? f.reason,
          }),
        })
      }
    }
  }
  if (enabled('coordination')) {
    const lastSeq = prev.activity.length ? prev.activity[prev.activity.length - 1].seq : 0
    for (const a of next.activity) {
      if (a.seq <= lastSeq) continue
      if (a.phase === 'stall_halt') {
        out.push({
          level: 'error',
          text: i18next.t('shell.coordinatorHalted', {
            id: a.coordinatorId,
            reason: a.reason || i18next.t('shell.phantomSpawn'),
          }),
        })
      }
    }
  }
  if (enabled('rota')) {
    for (const [id, t] of next.trajectories) {
      const before = prev.trajectories.get(id)
      if (!before && t.op === 'create') {
        out.push({
          level: 'info',
          text: i18next.t('shell.trajectoryStarted', {
            id,
            template: t.templateRef ? ' · ' + t.templateRef : '',
          }),
        })
        continue
      }
      if (before && before.status !== t.status && t.status && TRAJ_STATUS_TEXT[t.status]) {
        // abandoned/waiting are not failures but need the user's attention.
        const level: SignalLevel =
          t.status === 'done' ? 'success' : t.status === 'failed' ? 'error' : 'warning'
        out.push({ level, text: i18next.t(TRAJ_STATUS_TEXT[t.status], { id }) })
      }
    }
  }
  return out
}
