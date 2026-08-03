import type { Agent } from '@/types'

// A session outlives the agent that owned it, so any view that renders history
// can be handed an agent id that is deleted — or, for a store written before the
// agent existed / by a workspace since edited, one that resolves to nothing at
// all. Every call site used to invent its own answer: the sidebar skipped the
// avatar, the transcript printed the raw id, the empty state silently fell back
// to a DIFFERENT agent. resolveAgent is the one answer.

export const DELETED_AGENT_LABEL = 'Silinmiş ajan'

export interface ResolvedAgent {
  id: string
  name: string
  avatar?: string
  color?: string
  provider?: string
  model?: string
  /** The agent was deleted, or could not be resolved at all. */
  deleted: boolean
  /** No row exists for this id — the name is a placeholder, not the real one. */
  missing: boolean
}

/**
 * Resolve an agent id for RENDERING.
 *
 * Pass the full roster (`allAgents`, which includes deleted agents), not the
 * live subset — the live one cannot resolve a deleted author by definition.
 *
 * Returns null only for an empty id (no author to render, e.g. a user message).
 * An unknown id yields a placeholder marked `missing`, so the UI shows "deleted"
 * instead of leaking an internal id.
 */
export function resolveAgent(agents: Agent[], id: string | undefined | null): ResolvedAgent | null {
  if (!id) return null
  const found = agents.find((a) => a.id === id)
  if (!found) {
    return { id, name: DELETED_AGENT_LABEL, deleted: true, missing: true }
  }
  return {
    id: found.id,
    name: found.name,
    avatar: found.avatar,
    color: found.color,
    provider: found.provider,
    model: found.model,
    deleted: !!found.deleted,
    missing: false,
  }
}

/** Display name for an agent id — the real name where known, else the placeholder. */
export function agentName(agents: Agent[], id: string | undefined | null): string {
  return resolveAgent(agents, id)?.name ?? ''
}
