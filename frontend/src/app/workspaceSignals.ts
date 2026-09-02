// diffSignals — the pure half of useWorkspaceSignals: compares two lane-store
// snapshots and returns the toasts worth showing. Store-free so it runs in the
// node test environment.
import type { LaneState } from '@/shared/lib/laneModel'
import { SKIP_REASON_LABEL } from '@/features/schedules/fireMeta'

const TRAJ_STATUS_TEXT: Record<string, string> = {
  done: 'tamamlandı',
  failed: 'başarısız oldu',
  abandoned: 'terk edildi',
  waiting: 'bir insan yanıtı bekliyor',
}

// diffSignals compares two lane snapshots and returns the toasts to show —
// pure, so it is unit-testable without the store.
export function diffSignals(
  prev: LaneState,
  next: LaneState,
  enabled: (type: string) => boolean = () => true,
): { level: 'info' | 'success' | 'error'; text: string }[] {
  const out: { level: 'info' | 'success' | 'error'; text: string }[] = []
  if (enabled('automation')) {
    const lastSeq = prev.fires.length ? prev.fires[prev.fires.length - 1].seq : 0
    for (const f of next.fires) {
      if (f.seq <= lastSeq) continue
      const name = f.name || f.automationId
      if (f.outcome === 'fired') {
        out.push({
          level: 'info',
          text: `⚡ Otomasyon ${name} ateşlendi${f.sessionId ? ` → ${f.sessionId}` : ''}`,
        })
      } else if (f.reason && f.reason !== 'cooldown') {
        // Cooldown skips are routine noise; the others say a rule is stuck.
        out.push({
          level: 'info',
          text: `↷ Otomasyon ${name} atlandı: ${SKIP_REASON_LABEL[f.reason] ?? f.reason}`,
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
          text: `✕ Koordinatör ${a.coordinatorId} durduruldu: ${a.reason || 'phantom spawn'}`,
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
          text: `◈ Rota ${id} başladı${t.templateRef ? ` · ${t.templateRef}` : ''}`,
        })
        continue
      }
      if (before && before.status !== t.status && t.status && TRAJ_STATUS_TEXT[t.status]) {
        const level = t.status === 'done' ? 'success' : t.status === 'failed' ? 'error' : 'info'
        out.push({ level, text: `◈ Rota ${id} ${TRAJ_STATUS_TEXT[t.status]}` })
      }
    }
  }
  if (enabled('flow')) {
    for (const [id, r] of next.flowRuns) {
      const before = prev.flowRuns.get(id)
      if (before && before.status !== 'failure' && r.status === 'failure') {
        out.push({ level: 'error', text: `Akış koşusu ${id} başarısız: ${r.error || 'hata'}` })
      }
    }
  }
  return out
}
