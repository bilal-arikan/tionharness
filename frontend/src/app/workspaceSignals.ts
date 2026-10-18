import { i18next } from '@/i18n'
// diffSignals — the pure half of useWorkspaceSignals: compares two lane-store
// snapshots and returns the toasts worth showing. Store-free so it runs in the
// node test environment.
import type { LaneState } from '@/shared/lib/laneModel'
import { SKIP_REASON_LABEL } from '@/features/schedules/fireMeta'

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
  return out
}
