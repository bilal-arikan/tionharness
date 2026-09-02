import { toolBase } from './tools'

export type AgentActionTone = 'create' | 'stop' | 'message'

const CREATE_ACTIONS = new Set([
  'create_agent',
  'run_subagent',
  'spawn_agent',
  'spawn_session',
  'spawn_worker',
])

const STOP_ACTIONS = new Set(['close_agent', 'delete_agent', 'interrupt_session', 'stop_worker'])

const MESSAGE_ACTIONS = new Set(['send_input', 'send_message', 'send_to_worker'])

/** Classify agent lifecycle and communication steps without changing their labels. */
export function agentActionTone(tool: string, operation?: string): AgentActionTone | undefined {
  const action = operation || toolBase(tool)
  if (CREATE_ACTIONS.has(action)) return 'create'
  if (STOP_ACTIONS.has(action)) return 'stop'
  if (MESSAGE_ACTIONS.has(action)) return 'message'
  return undefined
}
