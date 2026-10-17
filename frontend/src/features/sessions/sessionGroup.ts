import { useCallback, useState } from 'react'
import type { Session } from '@/types'
import { isWorkerSession } from '@/shared/lib/coordination'

// The sessions column's two tabs. "main" holds the conversations a person
// started — plus the chats one of them spawned (spawn_session) or handed off to
// (/handoff) — and "other" holds every machine-driven run: coordinator workers,
// subagents, tasks, schedules, automations, insight scans, agent-to-agent message
// inboxes and system-agent work.
// Client twin of internal/api/sessions_groups.go: the server narrows the list by
// tab before paging, this predicate keeps rows merged in behind the filter (the
// open session) on the right tab.
export type SessionGroup = 'main' | 'other'

export const SESSION_GROUPS: SessionGroup[] = ['main', 'other']

// Tag a system agent's one-shot call carries (agent.SystemSessionTagPrefix);
// such a call is otherwise recorded as a plain user-origin chat.
const SYSTEM_SESSION_TAG_PREFIX = 'system:'

// Source id prefix of the per-agent inbox send_message deliveries append to
// (db.PeerThreadSourceID); otherwise a user-origin chat in every field.
const PEER_THREAD_SOURCE_PREFIX = 'agent-messages:'

const MAIN_ORIGINS = new Set(['user', 'spawn', 'handoff'])

// sessionGroupOf mirrors the server's sessionGroup. Kind is checked rather than
// the chip key because the backend stamps Category "chat" on new sessions of
// every kind, which would file a task or insight transcript as a chat.
export function sessionGroupOf(
  s: Pick<
    Session,
    | 'kind'
    | 'category'
    | 'executionType'
    | 'visibility'
    | 'tags'
    | 'sourceId'
    | 'origin'
    | 'role'
    | 'coordinatorSessionId'
  >,
): SessionGroup {
  if (s.kind !== '' && s.kind !== 'chat' && s.kind !== 'spawned') return 'other'
  if (isWorkerSession(s) || s.visibility === 'internal') return 'other'
  if (s.category && s.category !== 'chat') return 'other'
  if (s.executionType && s.executionType !== 'interactive') return 'other'
  if (s.tags?.some((tag) => tag.startsWith(SYSTEM_SESSION_TAG_PREFIX))) return 'other'
  if (s.sourceId?.startsWith(PEER_THREAD_SOURCE_PREFIX)) return 'other'
  // API responses always carry the origin (the backend derives it for old
  // headers); a missing one reads as a user chat, the derivation's fallback.
  return MAIN_ORIGINS.has(s.origin?.kind ?? 'user') ? 'main' : 'other'
}

// The tab is remembered across reloads, like the chip selection.
export const SESSION_GROUP_KEY = 'tionharness.sessionGroup'

export function normalizeSessionGroup(raw: string | null | undefined): SessionGroup {
  return raw === 'other' ? 'other' : 'main'
}

function readSessionGroup(): SessionGroup {
  try {
    return normalizeSessionGroup(globalThis.localStorage?.getItem(SESSION_GROUP_KEY))
  } catch {
    return 'main'
  }
}

// useSessionGroup owns the selected tab. It lives above the sidebar for the
// same reason the chips do: the session list request carries it.
export function useSessionGroup(): [SessionGroup, (group: SessionGroup) => void] {
  const [group, setGroupState] = useState<SessionGroup>(readSessionGroup)
  const setGroup = useCallback((next: SessionGroup) => {
    setGroupState(next)
    try {
      globalThis.localStorage?.setItem(SESSION_GROUP_KEY, next)
    } catch {
      // Persistence is best-effort when storage is blocked by browser policy.
    }
  }, [])
  return [group, setGroup]
}
