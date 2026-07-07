// useAppEvents owns the app-wide SSE subscription: autonomous-event handling
// (notifications, badges, live list refreshes, cross-window panel signals) and
// live turn-step frames. The subscription is mounted once; a deps ref refreshed
// each render keeps the handlers reading current state/closures — the same
// pattern the inline onEventRef/onStepRef handlers used inside App.tsx.
import { useEffect, useRef, type Dispatch, type MutableRefObject, type SetStateAction } from 'react'
import { api, getActiveWorkspace } from '@/api'
import type { AppEvent, Message, TurnStep } from '@/types'
import { isTypeEnabled } from '@/shared/lib/notifyPrefs'
import { notify } from '@/shared/lib/clientPrefs'
import type { useChatStream } from '@/features/chat/useChatStream'
import type { View } from './NavRail'
import { viewForEventType } from './eventViews'
import { bumpSignalsForEvent } from './eventToRefreshSignals'
import { routeFromEvent, buildRoute } from './url'
import type { ClientPrefs } from './useAppearance'

export interface AppEventDeps {
  chat: ReturnType<typeof useChatStream>
  view: View
  activeSessionId: string | null
  notifyEnabled: MutableRefObject<boolean>
  applyClientPrefs: (s: ClientPrefs) => void
  refreshWorkspaces: () => void
  refreshSessions: () => void
  setMessages: Dispatch<SetStateAction<Message[]>>
  setMeterRefresh: Dispatch<SetStateAction<number>>
  setSettingsNonce: Dispatch<SetStateAction<number>>
  markWorkspaceUnread: (id: string) => void
  markWorkspaceRead: (id: string) => void
  markViewUnread: (v: View) => void
}

// Autonomous-event handler: raise a desktop notification whose click deep-links
// to the event's target (chat session, board, or logs). Reads the deps snapshot
// refreshed each render, so it always sees current closures/state.
function onEvent(d: AppEventDeps, e: AppEvent) {
  // Application settings changed elsewhere — by an agent (update_settings) or
  // by another open window's Settings save: re-apply the client-side prefs
  // (theme/accent/notifications) live and signal the open Settings screen to
  // reload. App-global → no workspace badge, no toast.
  if (e.type === 'settings') {
    api.getSettings().then(d.applyClientPrefs).catch(() => {})
    d.setSettingsNonce((n) => n + 1)
    return
  }
  // The set of workspaces changed elsewhere — an agent created/renamed/deleted
  // one (list/create/rename/delete_workspace). Refresh the switcher list live.
  // App-global → no workspace badge, no toast.
  if (e.type === 'workspaces') {
    d.refreshWorkspaces()
    return
  }
  // An agent drove the UI here (focus_view). Apply the navigation immediately
  // — set the hash so the URL→state machinery switches workspace/view and
  // selects the entity — rather than waiting for a notification click. No
  // toast or badge: this IS the action, not a passive signal.
  if (e.type === 'navigate') {
    const r = routeFromEvent(e)
    if (r) window.location.hash = buildRoute(r)
    return
  }
  // An agent mutated this session's metadata (goal/title/working dir/archive
  // via the session tools). Refresh the session list (title/order/archived) and,
  // when it's the open session, bump the detail panel so its goal/title/cwd card
  // updates live. No toast — it's a quiet live-refresh signal.
  //
  // The op hint lets a subset of mutations also reload the active transcript
  // when something material changed inside it (a rewind, a deleted message, a
  // /summary or /handoff command landing, a new feedback rating). Plain
  // metadata edits (title/goal/workdir/pin/agent/role/tags/state) only touch
  // the row + the detail meter — no listMessages call needed.
  if (e.type === 'session') {
    const sid = e.target?.sessionId
    const op = e.target?.op
    d.refreshSessions()
    if (sid === d.activeSessionId) {
      d.setMeterRefresh((n) => n + 1)
      const transcriptOp = op === 'rewind' || op === 'delete_message' ||
        op === 'feedback' || op === 'summary' || op === 'handoff' || op === 'message_added'
      if (transcriptOp && sid) {
        api.listMessages(sid).then(d.setMessages).catch(() => {})
      }
    }
    return
  }
  // Badge any non-active workspace that produced activity (incl. completed
  // chats), so the switcher shows where to look. markWorkspaceUnread persists
  // the badge so every other open window picks it up via its storage listener.
  if (e.workspaceId && e.workspaceId !== getActiveWorkspace()) {
    d.markWorkspaceUnread(e.workspaceId)
  }
  // Same-workspace activity (chat/schedule) updates the session list
  // so unread dots, ordering and times stay live without a manual refresh.
  if (!e.workspaceId || e.workspaceId === getActiveWorkspace()) {
    // This window is live-viewing the workspace → the activity has been seen;
    // clear its badge across all windows (a different window may have set it).
    if (e.workspaceId) d.markWorkspaceRead(e.workspaceId)
    // Generic per-view unread: badge the event's nav view unless it's already
    // the one on screen in this window (then the user is seeing it live).
    const evView = viewForEventType(e.type)
    if (evView && evView !== d.view) d.markViewUnread(evView)
    d.refreshSessions()
    // A chat reply that completed server-side after the SSE stream closed
    // (e.g. the user refreshed mid-turn and the detached turn finished) is not
    // in the open transcript. If it belongs to the session being viewed,
    // reload its messages so the reply appears without a manual reselect.
    const sid = e.target?.sessionId
    if (e.type === 'chat' && sid) {
      // A self-wake (schedule_wake) has a lifecycle expressed via phase:
      //  - armed     → the turn ended into a WAITING state; raise the waiting
      //                banner (reason + fireAt) so the session doesn't look done.
      //  - start     → the wake fired; a server-driven turn began with no local
      //                run handle → drop the banner, raise the thinking indicator.
      //  - cancelled → the wake was disarmed → clear the banner.
      //  - done/other→ the turn ended → clear both.
      // Either way reload the transcript so the new prompt / reply appears.
      const phase = e.target?.phase
      if (phase === 'armed') {
        const fireAt = Number(e.target?.fireAt ?? 0) || 0
        d.chat.setWakeWait(sid, e.target?.reason ?? '', fireAt)
        d.chat.clearPending(sid)
      } else if (phase === 'start') {
        d.chat.clearWakeWait(sid)
        d.chat.markPending([sid])
      } else if (phase === 'cancelled') {
        d.chat.clearWakeWait(sid)
        d.chat.clearPending(sid)
      } else {
        // A generic chat event (e.g. the schedule_wake turn ending right after it
        // armed the wake) must NOT clear the waiting banner — otherwise the turn-end
        // event wipes it the instant it appears, and the session looks "done" while
        // a wake is still pending. Keep the banner until the wake fires (start) or
        // is cancelled; here only drop the thinking indicator.
        d.chat.clearPending(sid)
      }
      if (sid === d.activeSessionId) {
        d.chat.clearAutoLive(sid)
        api.listMessages(sid).then(d.setMessages).catch(() => {})
      }
    }
    // Autonomous turn completion (spawn / coordinator worker / scheduled run):
    // like the chat branch, drop the live ghost bubble and reload the transcript
    // so the authoritative persisted turn (with its full trace) replaces it. These
    // types carry no wake-phase logic — a plain reload is enough.
    if ((e.type === 'spawned' || e.type === 'worker' || e.type === 'schedule') && sid) {
      d.chat.clearPending(sid)
      if (sid === d.activeSessionId) {
        d.chat.clearAutoLive(sid)
        api.listMessages(sid).then(d.setMessages).catch(() => {})
      }
    }
  }
  // Cross-window panel refresh: every event that closes the workspace-match
  // gate above may move rows / status / memberships inside one or more
  // panels (TaskBoard, NetworkPanel, ExecutionsPanel, useActivity, ...). We
  // hand the event to a central mapper that returns the set of signal keys
  // panels subscribe to (board / network / activity / executions / agents /
  // flows / schedules / artifacts) and bump each with a 200ms per-key
  // debounce so a burst of events collapses into a single re-fetch per
  // panel. Placed AFTER the workspace-match block so cross-workspace events
  // (which only fire the badge / toast side) don't trigger a wasted GET
  // here — the same gate the chat/session logic already uses.
  bumpSignalsForEvent(e)
  // Chat completions only drive the badge (the streaming turn already raises
  // its own reply notification); other event types raise a desktop
  // notification that deep-links to the target on click.
  if (e.type === 'chat') return
  // Raise an OS toast only when the master toggle is on AND this event type is
  // not muted in Settings (per-type preference, device-local).
  if (!isTypeEnabled(e.type)) return
  // Tag the toast with the event identity so multiple open tabs/windows
  // (each receiving the same SSE event) collapse into a single OS toast
  // instead of one per tab.
  const tag = `${e.workspaceId ?? ''}:${e.type}:${e.target?.sessionId ?? e.target?.agentId ?? ''}:${e.time}`
  notify(d.notifyEnabled.current, e.title, e.body, () => {
    // Navigate via the deep-link URL: setting the hash drives the URL→state
    // machinery (useUrlSync → applyRoute), which switches workspace and
    // selects the entity correctly even across workspaces. notify() has
    // already focused the window.
    const r = routeFromEvent(e)
    if (r) window.location.hash = buildRoute(r)
  }, tag)
}

// Live turn-activity frames (session_step): fold each step into the active
// session's ghost bubble so an autonomous turn (scheduler/spawn/worker/wake) or
// the same chat turn viewed in another window renders its thinking/tool steps
// live. Ignore frames from other workspaces (their session isn't on screen here).
function onStep(d: AppEventDeps, e: AppEvent) {
  if (e.workspaceId && e.workspaceId !== getActiveWorkspace()) return
  const sid = e.target?.sessionId
  if (!sid || !e.step) return
  d.chat.applyAutoStep(sid, e.step as TurnStep)
}

export function useAppEvents(deps: AppEventDeps) {
  // Latest deps snapshot, refreshed after each render so the once-mounted SSE
  // subscription always navigates with current state/closures. (Effect-time
  // assignment: SSE frames arrive async, always after the effect has flushed.)
  const depsRef = useRef(deps)
  useEffect(() => {
    depsRef.current = deps
  })

  // Subscribe once to the global feed: notifications (onEvent) + live turn steps
  // (onStep). Both ride one EventSource; the ref keeps closures current.
  useEffect(
    () => api.subscribeEvents((e) => onEvent(depsRef.current, e), (e) => onStep(depsRef.current, e)),
    [],
  )
}
