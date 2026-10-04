import type { Agent } from '@/types'

/** Worker profiles are seeded with a `subagent-<profile>` system key (see
 * internal/agent/systemagents.go). The prefix is what separates the profiles
 * spawn_worker/run_subagent picks from, from the service agents the app runs
 * for itself (titler, compaction, insight, …). */
const WORKER_KEY_PREFIX = 'subagent-'

export interface SystemRosterGroups {
  /** Workspace customisations of the agents the app uses for its own jobs. */
  services: Agent[]
  /** Workspace customisations of the worker profiles. */
  workers: Agent[]
}

/** True when the agent serves a worker-profile system role. A customisation
 * carries the same systemKey as the built-in it is bound to, so it lands in the
 * same group without a second lookup. */
export function isWorkerSystemAgent(agent: Agent): boolean {
  return (agent.systemKey ?? '').startsWith(WORKER_KEY_PREFIX)
}

/** True for a locked built-in system agent — the compiled registry rows the
 * workspace roster leaves out (they stay reachable as inheritance parents and
 * through deep links). */
export function isBuiltinSystemAgent(agent: Agent): boolean {
  return !!agent.system && !!agent.locked
}

/** Splits the workspace's system-role customisations (the agents that inherit
 * from a built-in) into roster sections. The locked built-ins themselves are
 * not listed; they only fix the order, so customisations of the same role stay
 * adjacent in registry order. */
export function groupSystemAgents(agents: Agent[]): SystemRosterGroups {
  const builtins = agents.filter(isBuiltinSystemAgent)
  const custom = agents.filter((a) => a.system && !a.locked)
  const groups: SystemRosterGroups = { services: [], workers: [] }
  const bucket = (agent: Agent) => (isWorkerSystemAgent(agent) ? groups.workers : groups.services)
  const placed = new Set<string>()
  for (const b of builtins) {
    for (const c of custom) {
      if (c.systemKey === b.systemKey && !placed.has(c.id)) {
        bucket(c).push(c)
        placed.add(c.id)
      }
    }
  }
  // Legacy: a customisation whose built-in is not seeded yet.
  for (const c of custom) if (!placed.has(c.id)) bucket(c).push(c)
  return groups
}
