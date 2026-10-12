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

// roleCandidates lists the agents that may run `key`: only the role's own system
// agents — the locked built-in and the customisations derived from it. Ordinary
// agents are never offered (nor accepted by the server): a background job must
// not run on an agent the user did not set up for it. The saved value stays
// visible even when it is no longer eligible, so the picker does not go blank.
export function roleCandidates(agents: Agent[], key: string, currentId = ''): Agent[] {
  return agents.filter(
    (a) => a.id === currentId || (!!a.system && a.systemKey === key && !a.deleted && !a.disabled),
  )
}

// builtinRoleAgent is what runs the role when nothing is assigned: an enabled
// customisation wins over the locked built-in, mirroring ResolveSystemAgent.
export function builtinRoleAgent(agents: Agent[], key: string): Agent | undefined {
  const own = agents.filter((a) => a.systemKey === key && !a.disabled && !a.deleted)
  return own.find((a) => !a.locked) ?? own[0]
}
