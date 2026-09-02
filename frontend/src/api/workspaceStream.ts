// Workspace event stream client — the per-workspace twin of sessionStream
// (GET /api/workspace/stream, _Docs/77 R3). One ordered, replayable log of
// structured lifecycle facts for the active workspace: sessions created /
// changed state / deleted, flow runs started / waiting / finished, schedules
// armed for a future fire, automation fires, trajectory revisions. A live
// workspace view keeps an incremental picture from it and gap-fills after a
// reconnect instead of re-fetching everything.
//
// Contract differences from the session stream: a fresh subscribe replays
// nothing (load the current picture over REST, then apply live events from
// `head`), and `sessionId` on the hub event is empty — the subject is in the
// payload's `target` / `data`.
//
// The kinds, payload types and dataOf() live in workspaceEvents.ts (transport
// free) and are re-exported here so existing imports keep working.
import { subscribeHubStream } from './hubStream'
import type { WorkspaceHubEvent, WorkspaceStreamHandlers } from './workspaceEvents'

export * from './workspaceEvents'

// subscribeWorkspaceStream opens the active workspace's stream and keeps it
// alive across reconnects. Returns an unsubscribe function.
export function subscribeWorkspaceStream(handlers: WorkspaceStreamHandlers): () => void {
  return subscribeHubStream('/api/workspace/stream', {
    ...handlers,
    onEvent: (ev) => handlers.onEvent(ev as WorkspaceHubEvent),
  })
}
