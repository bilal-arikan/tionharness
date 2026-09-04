import type { Agent } from '@/types'

/** Worker profiles are seeded with a `subagent-<profile>` system key (see
 * internal/agent/systemagents.go). The prefix is what separates the profiles
 * spawn_worker/run_subagent picks from, from the service agents the app runs
 * for itself (titler, compaction, insight, …). */
const WORKER_KEY_PREFIX = 'subagent-'

export interface SystemRosterGroups {
  /** Built-in agents the app uses for its own jobs, plus their customisations. */
  services: Agent[]
  /** Worker profiles, plus their customisations. */
  workers: Agent[]
}

/** True when the agent serves a worker-profile system role. A customisation
 * carries the same systemKey as the built-in it is bound to, so it lands in the
 * same group without a second lookup. */
export function isWorkerSystemAgent(agent: Agent): boolean {
  return (agent.systemKey ?? '').startsWith(WORKER_KEY_PREFIX)
}

/** Splits the system agents into roster sections. Within each section rows stay
 * GROUPED: every locked built-in is followed by the workspace customisations
 * bound to its role, so "which row serves this role" reads top-down. */
export function groupSystemAgents(agents: Agent[]): SystemRosterGroups {
  const builtins = agents.filter((a) => a.system && a.locked)
  const custom = agents.filter((a) => a.system && !a.locked)
  const groups: SystemRosterGroups = { services: [], workers: [] }
  const bucket = (agent: Agent) => (isWorkerSystemAgent(agent) ? groups.workers : groups.services)
  const placed = new Set<string>()
  for (const b of builtins) {
    const rows = bucket(b)
    rows.push(b)
    for (const c of custom) {
      if (c.systemKey === b.systemKey) {
        rows.push(c)
        placed.add(c.id)
      }
    }
  }
  // Legacy: a customisation whose built-in is not seeded yet.
  for (const c of custom) if (!placed.has(c.id)) bucket(c).push(c)
  return groups
}
