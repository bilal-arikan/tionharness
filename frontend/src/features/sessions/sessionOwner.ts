import type { Agent, Session } from '@/types'
import { resolveAgent, type ResolvedAgent } from '@/shared/lib/agentLookup'

// Who a session row belongs to, for rendering.
//
// A plain chat / spawn / worker session carries `agentId`, so `resolveAgent` on
// its own was enough. A DELEGATED run (run_subagent) does not: it is created
// against a target, so its identity lives in `targetAgentId` (an existing agent)
// or `targetProfile` (a built-in profile such as "coder", run through an
// ephemeral clone of the caller that is never persisted and therefore has no id
// to resolve). Reading only `agentId` left those rows anonymous — no avatar, no
// name — which is exactly the case this module exists to cover.
//
// Order: the owning agent first, then the delegation target, then the profile.

/** Label shown for a delegated run against a built-in profile. */
export function profileOwnerLabel(profile: string): string {
  return `subagent:${profile.trim()}`
}

/**
 * Resolve the agent to show for a session row.
 *
 * Returns null only when the session names no owner at all — neither an agent
 * nor a delegation target nor a profile.
 */
export function resolveSessionOwner(agents: Agent[], session: Session): ResolvedAgent | null {
  const direct = resolveAgent(agents, session.agentId || session.targetAgentId)
  if (direct) return direct
  const profile = (session.targetProfile ?? '').trim()
  if (!profile) return null
  // An ephemeral profile subagent has no agent row by design, so this is a real
  // identity rather than a missing one: name it, but carry no avatar or colour.
  return {
    id: '',
    name: profileOwnerLabel(profile),
    deleted: false,
    missing: false,
  }
}

/**
 * The label for a session row.
 *
 * Falls back for a row the backend never named. "Yeni sohbet" is right for an
 * untitled chat the user just opened, but wrong for a delegated run written
 * before those carried a title: naming it after the agent it ran keeps the row
 * identifiable instead of reading like an empty chat.
 */
export function sessionRowLabel(agents: Agent[], session: Session): string {
  const title = (session.title ?? '').trim()
  if (title) return title
  const owner = resolveSessionOwner(agents, session)
  if (session.kind === 'subagent' && owner) return owner.name
  return 'Yeni sohbet'
}
