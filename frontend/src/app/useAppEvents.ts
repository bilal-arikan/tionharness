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
import { playTurnDone } from '@/shared/lib/sounds'
import type { useChatStream } from '@/features/chat/useChatStream'
import type { View } from './NavRail'
import { viewForEventType } from './eventViews'
import { bumpSignalsForEvent } from './eventToRefreshSignals'
import { publishStep, publishTurnEnd } from '@/shared/lib/stepBus'
import { publishFlowNode } from '@/shared/lib/flowNodeBus'
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
      // Fan the turn-end out on the shared step bus for any other transcript
      // consumer of this session, regardless of which session the chat itself has
      // active. 'armed'/'start' phases are not ends.
      if (phase !== 'armed' && phase !== 'start') publishTurnEnd(sid)
      // Completion feedback when a real assistant reply lands: a turn-done chime
      // (gated by the device-local sound-effects pref) plus, when this window is
      // backgrounded, an OS toast that deep-links to the session on click. The
      // wake lifecycle phases (armed/start/cancelled) are NOT completions.
      if (!phase || phase === 'done') {
        playTurnDone()
        notify(
          d.notifyEnabled.current,
          e.title || 'Yanıt hazır',
          e.body || '',
          () => {
            const r = routeFromEvent(e)
            if (r) window.location.hash = buildRoute(r)
          },
          `chat-done:${e.workspaceId ?? ''}:${sid}:${e.time}`,
        )
      }
    }
    // Autonomous turn completion (spawn / coordinator worker / scheduled run /
    // flow run / automation fire): like the chat branch, drop the live ghost
    // bubble and reload the transcript so the authoritative persisted turn (with
    // its full trace) replaces it. These types carry no wake-phase logic — a
    // plain reload is enough. flow/automation are included because the chat
    // screen is now the unified transcript view: their sessions are selectable in
    // the sidebar, so a running one must land its finished turn without a manual
    // reselect. (Board task runs surface as 'spawned'/'chat' sessions; there is
    // no distinct 'task' run event.)
    if (
      (e.type === 'spawned' || e.type === 'worker' || e.type === 'schedule' ||
        e.type === 'flow' || e.type === 'automation') &&
      sid
    ) {
      d.chat.clearPending(sid)
      if (sid === d.activeSessionId) {
        d.chat.clearAutoLive(sid)
        api.listMessages(sid).then(d.setMessages).catch(() => {})
      }
      // Same turn-end fan-out as the chat branch (see above): unconditional so a
      // transcript view showing this session hears it even off the chat screen.
      publishTurnEnd(sid)
    }
  }
  // Cross-window panel refresh: every event may move rows / status /
  // memberships inside one or more panels (TaskBoard, NetworkPanel,
  // ExecutionsPanel, useActivity, ...). We hand the event to a central mapper
  // that returns the set of signal keys panels subscribe to (board / network /
  // activity / executions / agents / flows / schedules / artifacts) and bump
  // each with a 200ms per-key debounce so a burst of events collapses into a
  // single re-fetch per panel.
  //
  // GATED on the same workspace-match as the chat/session logic above: the
  // mounted panels only ever hold the ACTIVE workspace's data (they remount +
  // re-fetch per workspace), and every panel endpoint is workspace-scoped, so a
  // cross-workspace event bumping these keys would only trigger a wasted GET
  // that returns this workspace's unchanged data — and briefly light indicators
  // (e.g. activity) for another workspace's work. Cross-workspace events still
  // fire their badge / toast side below.
  if (!e.workspaceId || e.workspaceId === getActiveWorkspace()) {
    bumpSignalsForEvent(e)
  }
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
  // Fan the same frame out to any other live transcript view (ExecutionsPanel
  // via useLiveTranscript) so it grows its own ghost bubble in lock-step with
  // the chat. One SSE feed, one step bus, N transcript views.
  publishStep(sid, e.step as TurnStep)
}

// Live flow-node frames (flow_node): fan each node lifecycle frame out to the
// run viewer showing that run (via flowNodeBus, keyed by target.flowRunId) so
// per-node progress renders live. Ignore frames from other workspaces.
function onFlowNode(_d: AppEventDeps, e: AppEvent) {
  if (e.workspaceId && e.workspaceId !== getActiveWorkspace()) return
  const runId = e.target?.flowRunId
  if (!runId || !e.node) return
  publishFlowNode(runId, e.node)
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
    () =>
      api.subscribeEvents(
        (e) => onEvent(depsRef.current, e),
        (e) => onStep(depsRef.current, e),
        (e) => onFlowNode(depsRef.current, e),
      ),
    [],
  )
}
