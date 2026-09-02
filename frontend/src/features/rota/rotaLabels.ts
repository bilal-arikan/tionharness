// Small pure label helpers for the Rota skeleton rows.
import type { LaneLive, LaneSession } from '@/shared/lib/laneModel'

const LIVE_LABEL: Record<string, string> = {
  running: 'çalışıyor',
  queued: 'kuyrukta',
  waiting_ask: 'soru bekliyor',
  waiting_input: 'girdi bekliyor',
  awaiting_workers: "worker'ları bekliyor",
}

// laneLiveLabel renders a liveness entry as a short Turkish badge (with the
// queue depth when turns are waiting), or '' when the session is idle.
export function laneLiveLabel(live?: LaneLive): string {
  if (!live) return ''
  const base = LIVE_LABEL[live.state] ?? live.state
  return live.waiting && live.waiting > 0 ? `${base} +${live.waiting}` : base
}

// laneOriginGlyph marks how a session came to be, from its origin kind.
export function laneOriginGlyph(s: LaneSession): string {
  if (s.coordinator) return '◎'
  switch (s.origin?.kind) {
    case 'coordinator':
    case 'subagent':
      return '↳'
    case 'flow':
      return '⇶'
    case 'schedule':
      return '⏰'
    case 'automation':
      return '⚡'
    case 'handoff':
      return '↪'
    case 'spawn':
      return '⤴'
    case 'insight':
      return '🔎'
    default:
      return '●'
  }
}
