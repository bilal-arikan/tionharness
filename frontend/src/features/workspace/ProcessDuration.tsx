import { useSyncExternalStore } from 'react'
import type { ProcessEntry } from '@/types'
import { formatDurationMs } from '@/shared/lib/time'

// One shared one-second clock for every running row. Only the duration cells of
// running processes subscribe, so a tick re-renders those cells instead of the
// whole (up to 300-row) table, and every running row still moves on the same
// instant. The clock stops while the tab is hidden and when no cell listens.
const listeners = new Set<() => void>()
let now = Date.now()
let timer: ReturnType<typeof setInterval> | null = null

function tick() {
  now = Date.now()
  for (const l of listeners) l()
}

function sync() {
  const want = listeners.size > 0 && document.visibilityState !== 'hidden'
  if (want && !timer) {
    tick()
    timer = setInterval(tick, 1000)
  } else if (!want && timer) {
    clearInterval(timer)
    timer = null
  }
}

function subscribe(listener: () => void) {
  if (listeners.size === 0) document.addEventListener('visibilitychange', sync)
  listeners.add(listener)
  sync()
  return () => {
    listeners.delete(listener)
    if (listeners.size === 0) document.removeEventListener('visibilitychange', sync)
    sync()
  }
}

const getNow = () => now

// elapsedMs is how long the entry has been running, or how long it ran.
function elapsedMs(e: ProcessEntry, at: number): number {
  const end = e.endedAt && e.endedAt > 0 ? e.endedAt : at
  return Math.max(0, end - e.startedAt)
}

function RunningDuration({ entry }: { entry: ProcessEntry }) {
  const at = useSyncExternalStore(subscribe, getNow, getNow)
  return <>{formatDurationMs(elapsedMs(entry, at))}</>
}

// ProcessDuration renders the duration cell's text: a ticking value for a
// running process, a fixed one for a finished process.
export function ProcessDuration({ entry }: { entry: ProcessEntry }) {
  if (entry.status === 'running') return <RunningDuration entry={entry} />
  return <>{formatDurationMs(elapsedMs(entry, getNow()))}</>
}
