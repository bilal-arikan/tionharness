import type { Agent } from '@/types'

// System roles a workspace can assign an agent to (ws-settings
// systemAgentAssignments, internal/agent/systemagent_assign.go). Grouped the way
// the screens show them; each screen picks the groups or single roles it owns.
export const SYSTEM_ROLE_GROUPS = {
  workers: [
    'subagent-explore',
    'subagent-planner',
    'subagent-coder',
    'subagent-reviewer',
    'subagent-validator',
    'subagent-config',
  ],
  helpers: ['titler', 'compaction', 'overview-summarizer'],
  analysis: ['insight', 'lesson-extractor'],
  optimizers: ['flow-optimizer', 'recipe-optimizer', 'stall-judge'],
} as const

export type SystemRoleGroup = keyof typeof SYSTEM_ROLE_GROUPS

export const isWorkerRole = (key: string) => key.startsWith('subagent-')

// roleCandidates lists the agents that may run `key`: any live regular agent, or
// the role's own built-in/customisation. Another role's system agent is refused
// by the server, so it is never offered. The saved value stays visible even when
// it is no longer eligible, so the picker does not silently go blank.
export function roleCandidates(agents: Agent[], key: string, currentId = ''): Agent[] {
  return agents.filter(
    (a) => a.id === currentId || (!a.deleted && !a.disabled && (!a.system || a.systemKey === key)),
  )
}

// builtinRoleAgent is what runs the role when nothing is assigned: an enabled
// customisation wins over the locked built-in, mirroring ResolveSystemAgent.
export function builtinRoleAgent(agents: Agent[], key: string): Agent | undefined {
  const own = agents.filter((a) => a.systemKey === key && !a.disabled && !a.deleted)
  return own.find((a) => !a.locked) ?? own[0]
}
