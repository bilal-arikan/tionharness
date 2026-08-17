import type { BadgeTone } from '@/shared/components/Badge'

// Run-outcome display metadata. `runState` is how a session's LAST background work
// turn ENDED (db.Session.RunState, runtime-owned and read-only). It is a different
// axis from `state`, which is the two-valued visibility field the sidebar tabs
// filter on: an archived session may have completed, and a failed one is usually
// still active. Nothing here touches that filter.
//
// Kept in its own module (importing only the Badge tone type) so it stays free of
// the shared component barrel, which transitively reaches the API client and its
// localStorage access — that would make this untestable under the repo's node-env
// vitest setup.
const RUN_STATE_META: Record<string, { label: string; tone: BadgeTone; title: string }> = {
  completed: { label: 'tamamlandı', tone: 'muted', title: 'Son arka plan turu tamamlandı' },
  failed: { label: 'başarısız', tone: 'danger', title: 'Son arka plan turu hatayla bitti' },
  killed: { label: 'durduruldu', tone: 'warning', title: 'Son arka plan turu durduruldu' },
  timeout: {
    label: 'zaman aşımı',
    tone: 'warning',
    title: 'Son arka plan turu zaman aşımına uğradı',
  },
  // The runtime also writes 'incomplete' (a turn that ran out of budget with work
  // left). It reads as an outcome, not a fault, so it gets the quiet accent tone.
  incomplete: { label: 'yarım kaldı', tone: 'accent', title: 'Son arka plan turu yarım kaldı' },
}

// runStateMeta resolves the badge to render for a session's run outcome, or
// undefined when nothing should be shown.
//
// Empty means the session never ran a background turn — including every session
// written before the field existed — so it renders NO badge rather than a
// misleading "completed". The three bad outcomes stay distinguishable from each
// other (they call for different actions: retry vs. it-was-stopped vs. give-it-
// longer) while `completed` is deliberately quiet, since most rows carry it.
export function runStateMeta(runState?: string) {
  if (!runState) return undefined
  return RUN_STATE_META[runState]
}
