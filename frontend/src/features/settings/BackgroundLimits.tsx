import { NumberField } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function BackgroundLimits({ draft, set }: PanelProps) {
  return (
    <>
      <p className="text-xs text-[var(--color-text-dim)]">
        Productive work has no total duration cap. Idle windows detect stalled runs.
      </p>
      <NumberField
        label="Concurrent background sessions"
        hint="Maximum background sessions running at the same time (1–128)."
        min={1}
        max={128}
        value={draft.spawnMaxConcurrent}
        onChange={(v) => set('spawnMaxConcurrent', v)}
      />
      <NumberField
        label="Queued background sessions"
        hint="Maximum sessions waiting for available capacity (1–128)."
        min={1}
        max={128}
        value={draft.spawnQueueMax}
        onChange={(v) => set('spawnQueueMax', v)}
      />
      <NumberField
        label="Completed flow runs to retain"
        hint="Completed run trees kept per flow. 0 means unlimited; older runs are removed periodically."
        min={0}
        max={1000}
        value={draft.flowRunRetention}
        onChange={(v) => set('flowRunRetention', v)}
      />
      <NumberField
        label="Background sessions per turn"
        hint="Maximum sessions an agent can start in one turn (1–64)."
        min={1}
        max={64}
        value={draft.spawnMaxPerTurn}
        onChange={(v) => set('spawnMaxPerTurn', v)}
      />
      <NumberField
        label="Background idle window (minutes)"
        hint="Cancel a background, worker or scheduled run after this long without meaningful progress. Productive runs have no total duration cap."
        min={1}
        max={1440}
        value={draft.spawnIdleTimeoutMin}
        onChange={(v) => set('spawnIdleTimeoutMin', v)}
      />
      <NumberField
        label="Chat idle window (minutes)"
        hint="Cancel an interactive turn with no meaningful steps. Network heartbeats do not count. 0 disables the limit."
        min={0}
        max={1440}
        value={draft.chatTurnIdleTimeoutMin}
        onChange={(v) => set('chatTurnIdleTimeoutMin', v)}
      />
      <NumberField
        label="Codex output idle window (seconds)"
        hint="Idle window for the Codex subprocess output stream."
        min={0}
        max={86400}
        value={draft.codexStdoutIdleSec}
        onChange={(v) => set('codexStdoutIdleSec', v)}
      />
      <NumberField
        label="Idle recovery attempts"
        hint="Maximum automatic resume attempts after an idle interruption."
        min={0}
        max={5}
        value={draft.idleResumeMax}
        onChange={(v) => set('idleResumeMax', v)}
      />
      <NumberField
        label="Turn watchdog window (minutes)"
        hint="Idle window used by the turn watchdog."
        min={1}
        max={1440}
        value={draft.turnIdleWatchdogMin}
        onChange={(v) => set('turnIdleWatchdogMin', v)}
      />
    </>
  )
}
