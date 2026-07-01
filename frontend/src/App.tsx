import { useEffect, useState, useCallback, useRef, useMemo, lazy, Suspense } from 'react'
import { PanelRight, ClipboardCopy, Check, FolderOpen } from 'lucide-react'
import { api, getActiveWorkspace, setActiveWorkspace } from './api'
import type { Agent, AgentPatch, Artifact, Session, Message, AppSettings, AppEvent } from './types'
import { NavRail, type View } from './components/NavRail'
import { SessionsSidebar } from './components/sessions/SessionsSidebar'
import { AgentRoster } from './components/agents/AgentRoster'
import { AgentsView } from './components/agents/AgentsView'
import { MessageList } from './components/chat/MessageList'
import { Composer } from './components/chat/Composer'
import { AskPrompt } from './components/chat/AskPrompt'
import { PermissionPrompt } from './components/chat/PermissionPrompt'
import { PlanPrompt } from './components/chat/PlanPrompt'
import { PendingTray } from './components/chat/PendingTray'
import { WakeWaitBanner } from './components/chat/WakeWaitBanner'
import { TodoPanel } from './components/chat/TodoPanel'
import { latestTodos } from './lib/todos'
import { TaskBoard } from './components/panels/TaskBoard'
import { Schedules } from './components/panels/Schedules'
import { MemoryPanel } from './components/panels/MemoryPanel'
// Code-split: the Flows panel pulls in React Flow (~300KB), loaded only when
// the user opens the Akışlar view.
const FlowsPanel = lazy(() =>
  import('./components/panels/FlowsPanel').then((m) => ({ default: m.FlowsPanel })),
)
// The collaboration network panel also pulls in React Flow — load it on demand.
const NetworkPanel = lazy(() =>
  import('./components/panels/NetworkPanel').then((m) => ({ default: m.NetworkPanel })),
)
import { ExecutionsPanel } from './components/panels/ExecutionsPanel'
import { ArtifactsPanel } from './components/panels/ArtifactsPanel'
import { ArtifactPreviewModal } from './components/artifacts/ArtifactPreviewModal'
import { SkillsPanel } from './components/panels/SkillsPanel'
import { MarketPanel } from './components/panels/MarketPanel'
import { BudgetPanel } from './components/panels/BudgetPanel'
import { ChatMeters } from './components/panels/ChatMeters'
import { SessionDetailPanel } from './components/sessions/SessionDetailPanel'
import { SettingsPanel } from './components/SettingsPanel'
import { WorkspaceView } from './components/workspace/WorkspaceView'
import { OnboardingScreen } from './components/workspace/OnboardingScreen'
import { SplashScreen } from './components/SplashScreen'
import { LogsPanel } from './components/panels/LogsPanel'
import { isTypeEnabled } from './lib/notifyPrefs'
import { useWorkspaces } from './hooks/useWorkspaces'
import { useActivity } from './hooks/useActivity'
import { useUnreadViews } from './hooks/useUnreadViews'
import { useUnreadBadge } from './hooks/useUnreadBadge'
import { useDirtyViews } from './lib/dirtySignals'
import { viewForEventType } from './lib/eventViews'
import { useChatStream } from './hooks/useChatStream'
import { useUrlSync } from './hooks/useUrlSync'
import { parseRoute, routeIdForView, routeFromEvent, buildRoute, type Route } from './lib/url'
import { isImagePath, mediaUrl } from './lib/paths'
import { applyAppearance, resolveAppearance, type Appearance } from './lib/theme'
import { applyKeepAwake, ensureNotificationPermission, notify } from './lib/clientPrefs'

// isChatKind reports whether a session is continuable from the chat sidebar.
// Manual chats (chat/empty) plus "spawned" sessions qualify: spawned covers
// both the spawn tool and handoff (context-reset) children, which are
// single-agent linear transcripts explicitly meant for a human to take over and
// keep talking to. Task/flow/schedule transcripts are aggregate/multi-run logs
// and stay read-only in the Activity (executions) view instead.
function isChatKind(kind: string): boolean {
  return kind === '' || kind === 'chat' || kind === 'spawned'
}

// Parse the deep-link once at module load. If it names a workspace, apply it to
// the api client immediately so useWorkspaces initialises on the routed
// workspace (an unknown id is validated away to the first workspace there).
const INITIAL_ROUTE: Route = parseRoute(window.location.hash)
if (INITIAL_ROUTE.workspaceId) setActiveWorkspace(INITIAL_ROUTE.workspaceId)

// Minimum time the first-run splash stays on screen (ms), so an instant workspace
// load doesn't flash the logo for a single frame. Only applies on a fresh install.
const SPLASH_MIN_MS = 1100

const VIEW_TITLE: Record<View, string> = {
  chat: 'Sohbet',
  executions: 'Aktivite',
  agents: 'Ajanlar',
  network: 'Ağ',
  board: 'Görevler',
  schedules: 'Zamanlamalar',
  memory: 'Hafıza',
  flows: 'Akışlar',
  artifacts: 'Artifactlar',
  skills: 'Skills',
  budget: 'Bütçe',
  logs: 'Loglar',
  market: 'Market',
  workspace: 'Workspace',
  settings: 'Ayarlar',
}

export default function App() {
  const [agents, setAgents] = useState<Agent[]>([])
  const [sessions, setSessions] = useState<Session[]>([])
  const [messages, setMessages] = useState<Message[]>([])
  const [activeAgentId, setActiveAgentId] = useState<string | null>(null)
  const [activeSessionId, setActiveSessionId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [view, setView] = useState<View>(INITIAL_ROUTE.view)
  const [meterRefresh, setMeterRefresh] = useState(0)
  // Deep-link target for the schedules screen (highlights the routed schedule).
  const [scheduleTarget, setScheduleTarget] = useState<string | null>(
    INITIAL_ROUTE.view === 'schedules' ? INITIAL_ROUTE.id : null,
  )
  // Active settings category (deep-link aware): #/w/{ws}/settings/{category}.
  const [settingsCat, setSettingsCat] = useState<string | null>(
    INITIAL_ROUTE.view === 'settings' ? INITIAL_ROUTE.id : null,
  )
  // Active workspace sub-tab (deep-link aware): #/w/{ws}/workspace/{tab}.
  const [workspaceTab, setWorkspaceTab] = useState<string | null>(
    INITIAL_ROUTE.view === 'workspace' ? INITIAL_ROUTE.id : null,
  )
  // Bumped whenever an agent changes app settings (the `settings` SSE event), so
  // an open Settings screen reloads to reflect the change.
  const [settingsNonce, setSettingsNonce] = useState(0)
  // Secrets moved under Settings as a sub-category: open the Settings screen
  // focused on the Secrets ("Sırlar") category.
  const openSecrets = () => {
    setSettingsCat('secrets')
    setView('settings')
  }
  // Entity selection carried by an initial/cross-workspace deep link, consumed
  // once by the workspace-load effect after agents+sessions arrive.
  const pendingRouteRef = useRef<Route | null>(INITIAL_ROUTE)
  const bumpMeter = useCallback(() => setMeterRefresh((n) => n + 1), [])

  const {
    workspaces,
    loading: wsLoading,
    activeWorkspaceId,
    unreadWs,
    favoriteWorkspaceId,
    setFavoriteWorkspace,
    markWorkspaceUnread,
    markWorkspaceRead,
    switchWorkspace,
    createWorkspace,
    deleteActiveWorkspace,
    deleteWorkspace,
    refreshWorkspaces,
  } = useWorkspaces(setError)

  // First-run gating. `hadSetupAtBoot` is captured ONCE at mount: a returning
  // user (who has had at least one workspace before) skips the splash entirely
  // and lands in the app shell, while a truly fresh install shows the splash
  // during the initial load and then the onboarding screen. The flag is set the
  // moment a workspace exists, so the splash never reappears after setup.
  const [hadSetupAtBoot] = useState(() => {
    try {
      return localStorage.getItem('swarmgo.hasSetup') === '1'
    } catch {
      return false
    }
  })
  useEffect(() => {
    if (workspaces.length > 0) {
      try {
        localStorage.setItem('swarmgo.hasSetup', '1')
      } catch {
        /* storage unavailable — non-fatal, splash logic just falls back to load timing */
      }
    }
  }, [workspaces.length])

  // Minimum splash duration: even if the workspace list resolves instantly, hold
  // the splash for SPLASH_MIN_MS so a fresh launch never flashes the logo for a
  // single frame. Returning users (hadSetupAtBoot) skip the splash entirely, so
  // the gate starts already-elapsed for them.
  const [minSplashElapsed, setMinSplashElapsed] = useState(hadSetupAtBoot)
  useEffect(() => {
    if (hadSetupAtBoot) return
    const t = setTimeout(() => setMinSplashElapsed(true), SPLASH_MIN_MS)
    return () => clearTimeout(t)
  }, [hadSetupAtBoot])

  // Right-hand session detail panel visibility (persisted).
  const [detailOpen, setDetailOpen] = useState(
    () => localStorage.getItem('swarmgo.detailOpen') === '1',
  )
  const toggleDetail = useCallback(() => {
    setDetailOpen((v) => {
      const next = !v
      localStorage.setItem('swarmgo.detailOpen', next ? '1' : '0')
      return next
    })
  }, [])
  // Header "copy path" feedback: briefly show a check after copying.
  const [pathCopied, setPathCopied] = useState(false)
  // Default agent for NEW sessions (chosen from the roster). Persisted so it
  // survives reloads; unmentioned turns in a session use the session's own agent.
  const [defaultAgentId, setDefaultAgentId] = useState<string | null>(
    () => localStorage.getItem('swarmgo.defaultAgentId'),
  )
  // Desktop-notification preference, read live in callbacks without re-binding.
  const notifyEnabled = useRef(false)
  // Latest autonomous-event handler, refreshed each render so the once-mounted
  // SSE subscription always navigates with current state/closures.
  const onEventRef = useRef<(e: AppEvent) => void>(() => {})
  // Artifact deep-link target: set when a chat artifact card is clicked, opening
  // the artifacts screen with that artifact pre-selected.
  const [artifactTarget, setArtifactTarget] = useState<string | null>(
    INITIAL_ROUTE.view === 'artifacts' ? INITIAL_ROUTE.id : null,
  )
  // Flow deep-link target: set when an Activity flow execution links to its flow,
  // opening the Flows screen on that flow's run history.
  const [flowTarget, setFlowTarget] = useState<string | null>(null)
  const openFlowRun = useCallback((flowId: string) => {
    setFlowTarget(flowId)
    setView('flows')
  }, [])
  // Executions deep-link target (sessionId): set when a schedule/flow notification
  // is clicked, opening the Activity feed with that run pre-selected.
  const [executionTarget, setExecutionTarget] = useState<string | null>(
    INITIAL_ROUTE.view === 'executions' ? INITIAL_ROUTE.id : null,
  )
  // Open a specific run on the Activity screen (used by the agent activity rail).
  const openExecution = useCallback((sessionId: string) => {
    setExecutionTarget(sessionId)
    setView('executions')
  }, [])

  // Appearance is per-workspace: the app-global appearance is the inherited
  // default, and each workspace may override the theme preset. We keep both in
  // refs so a global-settings save and a workspace switch can each re-resolve
  // and re-apply the effective theme without racing each other.
  const globalAppearanceRef = useRef<Appearance>({ themePreset: 'violet-dark' })
  const wsAppearanceRef = useRef<Partial<Appearance> | null>(null)
  const applyResolvedTheme = useCallback(() => {
    applyAppearance(resolveAppearance(wsAppearanceRef.current, globalAppearanceRef.current))
  }, [])

  // Apply the client-side preferences carried by app settings. Theme resolution
  // honors the active workspace's override on top of these global defaults.
  const applyClientPrefs = useCallback((s: { themePreset?: string; keepAwake: boolean; desktopNotifications: boolean }) => {
    globalAppearanceRef.current = { themePreset: s.themePreset ?? '' }
    applyResolvedTheme()
    applyKeepAwake(s.keepAwake)
    ensureNotificationPermission(s.desktopNotifications)
    notifyEnabled.current = s.desktopNotifications
  }, [applyResolvedTheme])

  // onAppearanceSaved is invoked by the Settings "Görünüm" panel after it persists
  // the active workspace's appearance override, so App's ref + the live theme stay
  // in sync (a later global save must not clobber the workspace choice).
  const onAppearanceSaved = useCallback((a: Partial<Appearance>) => {
    wsAppearanceRef.current = a
    applyResolvedTheme()
  }, [applyResolvedTheme])

  // Load global settings once and apply theme + client-side behaviours.
  useEffect(() => {
    api.getSettings().then(applyClientPrefs).catch((e) => setError(e.message))
  }, [applyClientPrefs])

  // Re-theme whenever the active workspace changes: fetch that workspace's
  // appearance override and apply it on top of the global defaults.
  useEffect(() => {
    if (!activeWorkspaceId) return
    let cancelled = false
    api.getWorkspaceSettings()
      .then((w) => {
        if (cancelled) return
        wsAppearanceRef.current = { themePreset: w.themePreset }
        applyResolvedTheme()
      })
      .catch(() => {})
    return () => { cancelled = true }
  }, [activeWorkspaceId, applyResolvedTheme])

  // Load agents + ALL sessions whenever the active workspace changes (the chat
  // is session-based: sessions are listed flat, not nested under an agent).
  useEffect(() => {
    if (!activeWorkspaceId) return
    setAgents([])
    setSessions([])
    setMessages([])
    setActiveAgentId(null)
    setActiveSessionId(null)
    let cancelled = false
    Promise.all([api.listAgents(), api.listSessions()])
      .then(([ag, ss]) => {
        if (cancelled) return
        setAgents(ag)
        setSessions(ss)
        // Default selection: the most recent chat session (task/flow/schedule
        // transcripts live in the Activity view, not the chat sidebar).
        const firstChat = ss.find((s) => isChatKind(s.kind))
        let sid = firstChat ? firstChat.id : null
        let aid = firstChat ? firstChat.agentId : null
        // Honor a pending deep link (initial load or cross-workspace nav) once.
        const want = pendingRouteRef.current
        pendingRouteRef.current = null
        if (want) {
          if (want.view === 'chat' && want.id && ss.some((s) => s.id === want.id)) {
            sid = want.id
            aid = ss.find((s) => s.id === want.id)?.agentId ?? aid
          } else if (
            (want.view === 'agents' || want.view === 'memory') &&
            want.id &&
            ag.some((a) => a.id === want.id)
          ) {
            aid = want.id
          } else if (want.view === 'executions') {
            // The Activity feed loads its own list; just hand it the run to select.
            setExecutionTarget(want.id)
          }
        }
        setActiveSessionId(sid)
        setActiveAgentId(aid)
      })
      .catch((e) => {
        if (!cancelled) setError((e as Error).message)
      })
    return () => {
      cancelled = true
    }
  }, [activeWorkspaceId])

  // Keep the default agent (for new sessions) valid: fall back to the first
  // agent when unset or pointing at a removed agent.
  useEffect(() => {
    if (agents.length === 0) return
    if (!defaultAgentId || !agents.some((a) => a.id === defaultAgentId)) {
      setDefaultAgentId(agents[0].id)
    }
  }, [agents, defaultAgentId])

  // Clicking a file path: open images inline (new tab via the file server),
  // copy other paths to the clipboard as a best-effort action.
  const openFile = useCallback((path: string) => {
    if (isImagePath(path)) {
      window.open(mediaUrl(path), '_blank')
    } else {
      navigator.clipboard?.writeText(path).catch(() => {})
    }
  }, [])

  // Clicking an artifact card/chip anywhere (chat, activity): preview it in a
  // modal overlay — no navigation to the Artifacts screen. The modal offers a
  // shortcut to open the full screen for editing.
  const [previewArtifactId, setPreviewArtifactId] = useState<string | null>(null)
  const openArtifact = useCallback((id: string) => {
    setPreviewArtifactId(id)
  }, [])
  // Open the dedicated Artifacts screen on a specific artifact (from the preview
  // modal's "open in screen" shortcut).
  const openArtifactFull = useCallback((id: string) => {
    setPreviewArtifactId(null)
    setArtifactTarget(id)
    setView('artifacts')
  }, [])

  // When the active session changes, load its messages.
  useEffect(() => {
    if (!activeSessionId) {
      setMessages([])
      return
    }
    api.listMessages(activeSessionId).then(setMessages)
  }, [activeSessionId])

  // Artifacts offered by the composer's "#" picker so the user can include an
  // artifact's content in the next turn. ALL workspace artifacts are referencable
  // (not just ones produced in this session) — manually-created and other-session
  // artifacts have an empty/different sessionId and would otherwise never show.
  // Also used to resolve attachment chips in the transcript (by sourcePath).
  // Refreshed after each turn (meterRefresh) since a turn may create new artifacts.
  const [sessionArtifacts, setSessionArtifacts] = useState<Artifact[]>([])
  useEffect(() => {
    if (!activeWorkspaceId) {
      setSessionArtifacts([])
      return
    }
    api.listArtifacts().then(setSessionArtifacts).catch(() => {})
  }, [activeWorkspaceId, activeSessionId, meterRefresh])

  // Mirror the active session id into a ref so the once-mounted event handler
  // can tell whether an incoming chat completion belongs to the open transcript.
  const activeSessionIdRef = useRef<string | null>(null)
  useEffect(() => {
    activeSessionIdRef.current = activeSessionId
  }, [activeSessionId])

  // Mirror the current view so the SSE event handler can decide whether an
  // event's target view is already being shown (→ no unread badge).
  const viewRef = useRef<View>(view)
  viewRef.current = view

  // Mirror the open transcript into a ref so the chat hook's retry can read the
  // current messages (to find the user prompt behind a failed turn) without
  // re-binding its callbacks on every message update.
  const messagesRef = useRef<Message[]>(messages)
  messagesRef.current = messages

  // Reload the session list (fresh order, updated times, unread flags).
  const refreshSessions = useCallback(() => {
    api.listSessions().then(setSessions).catch(() => {})
  }, [])

  // Change which agent answers the active chat session (the composer's mandatory
  // agent dropdown — "@mention" routing was removed). Updates the local selection
  // immediately and persists it to the session so it survives a reload.
  const changeChatAgent = useCallback((id: string) => {
    setActiveAgentId(id)
    const sid = activeSessionIdRef.current
    if (!sid) return
    api
      .setSessionAgent(sid, id)
      .then(() => setSessions((prev) => prev.map((s) => (s.id === sid ? { ...s, agentId: id } : s))))
      .catch((e) => setError((e as Error).message))
  }, [])

  // Select a session: reflect its default agent and clear its unread flag.
  // When a cross-session search result is clicked, the target message id is
  // stashed here so MessageList scrolls to (and briefly highlights) it once the
  // session's transcript has loaded. Cleared after the scroll is consumed.
  const [scrollToMsgId, setScrollToMsgId] = useState<string | null>(null)

  // A freshly-created "new chat" that has received no message yet. If the user
  // leaves it (opens another session or a new chat) without ever sending anything,
  // it is auto-deleted on the way out so empty abandoned chats don't pile up.
  const freshEmptyRef = useRef<string | null>(null)

  // discardEmptyFresh deletes the tracked fresh session when it is the one being
  // left AND nothing was ever sent in it (its live transcript is empty). leavingId
  // is the session being navigated away from.
  const discardEmptyFresh = useCallback(
    (leavingId: string | null) => {
      const id = freshEmptyRef.current
      if (!id || id !== leavingId) return
      freshEmptyRef.current = null
      // A message was sent → it's a real conversation, keep it.
      if ((messagesRef.current ?? []).length > 0) return
      api.deleteSession(id).catch(() => {})
      setSessions((prev) => prev.filter((s) => s.id !== id))
    },
    [messagesRef],
  )

  const selectSession = useCallback(
    (id: string, messageId?: string) => {
      // Leaving the current session: clean it up if it was an unused new chat.
      if (id !== activeSessionIdRef.current) discardEmptyFresh(activeSessionIdRef.current)
      setActiveSessionId(id)
      setScrollToMsgId(messageId ?? null)
      const sess = sessions.find((s) => s.id === id)
      if (sess) setActiveAgentId(sess.agentId)
      // Optimistically clear unread, then persist on the backend.
      setSessions((prev) => prev.map((s) => (s.id === id ? { ...s, unread: false } : s)))
      api.markSessionRead(id).catch(() => {})
    },
    [sessions, discardEmptyFresh, activeSessionIdRef],
  )

  // ---- per-session actions (settings menu) ----
  const renameSession = useCallback(async (id: string, title: string) => {
    try {
      await api.setSessionTitle(id, title)
      setSessions((prev) => prev.map((s) => (s.id === id ? { ...s, title } : s)))
    } catch (e) {
      setError((e as Error).message)
    }
  }, [])

  // Archive / restore a session (the sidebar Active/Archived filter). Archiving
  // updates state locally so the row leaves the active list at once; when the
  // archived session is the open one, fall back to another active session.
  const setSessionArchived = useCallback(
    async (id: string, archived: boolean) => {
      try {
        await api.setSessionState(id, archived ? 'archived' : 'active')
        setSessions((prev) => {
          const next = prev.map((s) => (s.id === id ? { ...s, state: archived ? 'archived' : 'active' } : s))
          if (archived && activeSessionId === id) {
            const fallback = next.find((s) => s.id !== id && s.state !== 'archived')
            setActiveSessionId(fallback?.id ?? null)
            setActiveAgentId(fallback?.agentId ?? null)
          }
          return next
        })
      } catch (e) {
        setError((e as Error).message)
      }
    },
    [activeSessionId],
  )

  // Pin / unpin a session (sidebar). Optimistic; ListSessions floats pinned to top.
  const setSessionPinned = useCallback(async (id: string, pinned: boolean) => {
    setSessions((prev) => prev.map((s) => (s.id === id ? { ...s, pinned } : s)))
    try {
      await api.setSessionPinned(id, pinned)
      refreshSessions()
    } catch (e) {
      setError((e as Error).message)
    }
  }, [refreshSessions])

  const copySessionPath = useCallback(async (id: string) => {
    try {
      const { path } = await api.sessionPath(id)
      await navigator.clipboard?.writeText(path)
    } catch (e) {
      setError((e as Error).message)
    }
  }, [])

  const revealSession = useCallback(async (id: string) => {
    try {
      await api.revealSession(id)
    } catch (e) {
      setError((e as Error).message)
    }
  }, [])

  // Header shortcut: copy the active session's folder path (with a brief check).
  const copyActiveSessionPath = useCallback(async () => {
    if (!activeSessionId) return
    await copySessionPath(activeSessionId)
    setPathCopied(true)
    setTimeout(() => setPathCopied(false), 1500)
  }, [activeSessionId, copySessionPath])

  const deleteSession = useCallback(
    async (id: string) => {
      if (id === freshEmptyRef.current) freshEmptyRef.current = null
      try {
        await api.deleteSession(id)
        setSessions((prev) => {
          const next = prev.filter((s) => s.id !== id)
          if (activeSessionId === id) {
            setActiveSessionId(next[0]?.id ?? null)
            setActiveAgentId(next[0]?.agentId ?? null)
          }
          return next
        })
      } catch (e) {
        setError((e as Error).message)
      }
    },
    [activeSessionId],
  )

  // Delete a single message from the open session (prune a mistaken/test one).
  const deleteMessage = useCallback(
    async (id: string) => {
      const sid = activeSessionIdRef.current
      if (!sid) return
      // Confirmation is handled inline by the message's DeleteButton (🗑 → Sil).
      try {
        await api.deleteMessage(sid, id)
        setMessages((prev) => prev.filter((m) => m.id !== id))
        refreshSessions()
      } catch (e) {
        setError((e as Error).message)
      }
    },
    [refreshSessions],
  )

  // Rate an assistant turn (👍/👎). Optimistic: update the local message, then
  // persist; the new feedback rides into session.jsonl for the reflector/eval.
  const rateMessage = useCallback(async (id: string, rating: number) => {
    const sid = activeSessionIdRef.current
    if (!sid) return
    setMessages((prev) =>
      prev.map((m) =>
        m.id === id
          ? { ...m, feedback: rating === 0 ? undefined : { rating, at: Math.floor(Date.now() / 1000) } }
          : m,
      ),
    )
    try {
      await api.setMessageFeedback(sid, id, rating)
    } catch (e) {
      setError((e as Error).message)
    }
  }, [])

  // Autonomous-event handler: raise a desktop notification whose click deep-links
  // to the event's target (chat session, board, or logs). Refreshed each render
  // so the stable SSE subscription below always sees current closures/state.
  onEventRef.current = (e: AppEvent) => {
    // Application settings changed elsewhere — by an agent (update_settings) or
    // by another open window's Settings save: re-apply the client-side prefs
    // (theme/accent/notifications) live and signal the open Settings screen to
    // reload. App-global → no workspace badge, no toast.
    if (e.type === 'settings') {
      api.getSettings().then(applyClientPrefs).catch(() => {})
      setSettingsNonce((n) => n + 1)
      return
    }
    // The set of workspaces changed elsewhere — an agent created/renamed/deleted
    // one (list/create/rename/delete_workspace). Refresh the switcher list live.
    // App-global → no workspace badge, no toast.
    if (e.type === 'workspaces') {
      refreshWorkspaces()
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
    if (e.type === 'session') {
      refreshSessions()
      if (e.target?.sessionId === activeSessionIdRef.current) {
        setMeterRefresh((n) => n + 1)
      }
      return
    }
    // Badge any non-active workspace that produced activity (incl. completed
    // chats), so the switcher shows where to look. markWorkspaceUnread persists
    // the badge so every other open window picks it up via its storage listener.
    if (e.workspaceId && e.workspaceId !== getActiveWorkspace()) {
      markWorkspaceUnread(e.workspaceId)
    }
    // Same-workspace activity (chat/schedule) updates the session list
    // so unread dots, ordering and times stay live without a manual refresh.
    if (!e.workspaceId || e.workspaceId === getActiveWorkspace()) {
      // This window is live-viewing the workspace → the activity has been seen;
      // clear its badge across all windows (a different window may have set it).
      if (e.workspaceId) markWorkspaceRead(e.workspaceId)
      // Generic per-view unread: badge the event's nav view unless it's already
      // the one on screen in this window (then the user is seeing it live).
      const evView = viewForEventType(e.type)
      if (evView && evView !== viewRef.current) markViewUnread(evView)
      refreshSessions()
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
          chat.setWakeWait(sid, e.target?.reason ?? '', fireAt)
          chat.clearPending(sid)
        } else if (phase === 'start') {
          chat.clearWakeWait(sid)
          chat.markPending([sid])
        } else if (phase === 'cancelled') {
          chat.clearWakeWait(sid)
          chat.clearPending(sid)
        } else {
          // A generic chat event (e.g. the schedule_wake turn ending right after it
          // armed the wake) must NOT clear the waiting banner — otherwise the turn-end
          // event wipes it the instant it appears, and the session looks "done" while
          // a wake is still pending. Keep the banner until the wake fires (start) or
          // is cancelled; here only drop the thinking indicator.
          chat.clearPending(sid)
        }
        if (sid === activeSessionIdRef.current) {
          api.listMessages(sid).then(setMessages).catch(() => {})
        }
      }
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
    notify(notifyEnabled.current, e.title, e.body, () => {
      // Navigate via the deep-link URL: setting the hash drives the URL→state
      // machinery (useUrlSync → applyRoute), which switches workspace and
      // selects the entity correctly even across workspaces. notify() has
      // already focused the window.
      const r = routeFromEvent(e)
      if (r) window.location.hash = buildRoute(r)
    }, tag)
  }

  // Subscribe once to the global autonomous-event feed (task/schedule).
  useEffect(() => api.subscribeEvents((e) => onEventRef.current(e)), [])

  // Pick the default agent for NEW sessions (from the roster).
  const pickDefaultAgent = useCallback((id: string) => {
    setDefaultAgentId(id)
    localStorage.setItem('swarmgo.defaultAgentId', id)
  }, [])

  // Roster click: set it as the default agent (for new chats) and as the active
  // agent (so the Memory/Tools panels, which are agent-scoped, follow along).
  const pickAgent = useCallback(
    (id: string) => {
      setActiveAgentId(id)
      pickDefaultAgent(id)
    },
    [pickDefaultAgent],
  )

  // Focus an agent across agent-scoped views (Agents/Memory/Tools) without
  // changing which agent is the default for new chats.
  const focusAgent = useCallback((id: string) => {
    setActiveAgentId(id)
  }, [])

  // Open an agent's settings page (Agents view, that agent selected). Used by the
  // chat transcript so clicking an assistant's avatar/name jumps to its settings.
  const openAgentSettings = useCallback((id: string) => {
    setActiveAgentId(id)
    setView('agents')
  }, [])

  const createAgent = useCallback(
    async (name: string, soul: string, provider: string, model?: string) => {
      try {
        const agent = await api.createAgent({ name, soul, provider, model })
        setAgents((prev) => [agent, ...prev])
        pickDefaultAgent(agent.id)
      } catch (e) {
        setError((e as Error).message)
      }
    },
    [pickDefaultAgent],
  )

  const updateAgent = useCallback(async (id: string, patch: AgentPatch) => {
    const updated = await api.updateAgent(id, patch)
    setAgents((prev) => prev.map((a) => (a.id === id ? updated : a)))
  }, [])

  // Delete an agent (and its owned sessions); refresh the affected lists.
  const deleteAgent = useCallback(async (id: string) => {
    try {
      await api.deleteAgent(id)
      setAgents((prev) => prev.filter((a) => a.id !== id))
      // The agent's sessions were removed server-side; reload the list and drop
      // the active session if it belonged to the deleted agent.
      try {
        const fresh = await api.listSessions()
        setSessions(fresh)
        setActiveSessionId((cur) => (cur && fresh.some((s) => s.id === cur) ? cur : fresh[0]?.id ?? null))
      } catch { /* ignore */ }
    } catch (e) {
      setError((e as Error).message)
    }
  }, [])

  const newSession = useCallback(async () => {
    const aid = defaultAgentId ?? agents[0]?.id
    if (!aid) return
    // Discard the previous new chat if it was left empty, before opening another.
    discardEmptyFresh(activeSessionIdRef.current)
    const s = await api.createSession(aid)
    setSessions((prev) => [s, ...prev])
    setActiveSessionId(s.id)
    setActiveAgentId(s.agentId)
    setMessages([])
    // Track it as a fresh, unused chat (cleared once a message is sent / it's left).
    freshEmptyRef.current = s.id
  }, [defaultAgentId, agents, discardEmptyFresh, activeSessionIdRef])

  // Regenerate a session's title from its conversation on demand.
  const regenerateSessionTitle = useCallback(async (sessionId: string) => {
    try {
      const { title } = await api.generateSessionTitle(sessionId)
      setSessions((prev) =>
        prev.map((s) => (s.id === sessionId ? { ...s, title } : s)),
      )
    } catch (e) {
      setError((e as Error).message)
    }
  }, [])

  // Chat-turn streaming machinery (send loop, interventions, slash commands).
  const chat = useChatStream({
    agents,
    sessions,
    activeSessionId,
    activeAgentId,
    activeSessionIdRef,
    messagesRef,
    notifyEnabled,
    setMessages,
    setError,
    setView,
    selectSession,
    refreshSessions,
    bumpMeter,
  })

  // After a reload (or workspace switch), restore the "thinking" indicator for
  // any turn still running detached on the server — the user may have refreshed
  // right after sending. Each turn's chat-completion event clears it again.
  useEffect(() => {
    if (!activeWorkspaceId) return
    api.activeSessions().then(chat.markPending).catch(() => {})
  }, [activeWorkspaceId, chat.markPending])

  // The active session's current checklist (latest todo_write across the
  // transcript). Pinned above the composer and updated as the agent ticks items.
  const currentTodos = useMemo(() => latestTodos(messages), [messages])

  // Manual chats for the chat sidebar (other kinds live in the Activity view).
  const chatSessions = useMemo(() => sessions.filter((s) => isChatKind(s.kind)), [sessions])

  // Per-view "work in progress" flags for the nav-rail busy indicators.
  const busyViews = useActivity(activeWorkspaceId, chat.streamingSessions.size > 0)
  // Per-view unread (unseen activity from the SSE feed) + unsaved-edit (dirty)
  // signals — the other two channels of the generic nav notification system.
  const { unreadViews, markViewUnread, markViewRead } = useUnreadViews(activeWorkspaceId)
  const dirtyViews = useDirtyViews()

  // Clear a view's unread badge as soon as it is shown (in this window).
  useEffect(() => {
    markViewRead(view)
  }, [view, markViewRead])

  // Total unseen items, surfaced on the tab title (while unfocused) + OS taskbar
  // badge. Sum of: unread chat sessions (active ws) + other workspaces with
  // activity + non-chat view badges (chat is already counted via sessions).
  const unreadTotal = useMemo(() => {
    const sess = chatSessions.filter((s) => s.unread).length
    const views = [...unreadViews].filter((v) => v !== 'chat').length
    return sess + unreadWs.size + views
  }, [chatSessions, unreadViews, unreadWs])
  useUnreadBadge(unreadTotal)

  // Apply a Route (from back/forward, a manual URL edit, or a shared link) to
  // the app state. A workspace switch defers entity selection to the
  // workspace-load effect via pendingRouteRef; same-workspace navigation applies
  // the entity immediately.
  const applyRoute = useCallback(
    (r: Route) => {
      setView(r.view)
      if (r.workspaceId && r.workspaceId !== getActiveWorkspace()) {
        pendingRouteRef.current = r
        switchWorkspace(r.workspaceId)
        return
      }
      if (r.view === 'chat') {
        if (r.id) selectSession(r.id)
      } else if (r.view === 'agents' || r.view === 'memory') {
        if (r.id) focusAgent(r.id)
      } else if (r.view === 'artifacts') {
        setArtifactTarget(r.id)
      } else if (r.view === 'schedules') {
        setScheduleTarget(r.id)
      } else if (r.view === 'executions') {
        setExecutionTarget(r.id)
      } else if (r.view === 'settings') {
        setSettingsCat(r.id)
      } else if (r.view === 'workspace') {
        setWorkspaceTab(r.id)
      }
    },
    [switchWorkspace, selectSession, focusAgent],
  )

  // The canonical route for the current state, mirrored to the URL hash.
  const route: Route = {
    workspaceId: activeWorkspaceId,
    view,
    id: routeIdForView(view, {
      sessionId: activeSessionId,
      agentId: activeAgentId,
      artifactId: artifactTarget,
      scheduleId: scheduleTarget,
      settingsCat,
      workspaceTab,
      executionId: executionTarget,
    }),
  }
  useUrlSync(route, !!activeWorkspaceId, applyRoute)

  // ---- First-run gating (must stay AFTER every hook above) ----
  // Fresh install → splash while the list is loading AND until the minimum
  // duration elapses (only when no prior setup). Returning users skip it.
  if (!hadSetupAtBoot && (wsLoading || !minSplashElapsed)) {
    return <SplashScreen />
  }
  // List resolved and there are zero workspaces → onboarding. Closing the popup
  // without creating one provisions nothing (no default workspace).
  if (!wsLoading && workspaces.length === 0) {
    return <OnboardingScreen onCreate={createWorkspace} />
  }

  return (
    <div className="flex h-full">
      <NavRail
        view={view}
        onSelectView={setView}
        workspaces={workspaces}
        activeWorkspaceId={activeWorkspaceId}
        unreadWorkspaceIds={unreadWs}
        busyViews={busyViews}
        unreadViews={unreadViews}
        dirtyViews={dirtyViews as Set<View>}
        favoriteWorkspaceId={favoriteWorkspaceId}
        onSetFavoriteWorkspace={setFavoriteWorkspace}
        onSwitchWorkspace={switchWorkspace}
        onCreateWorkspace={createWorkspace}
        onDeleteWorkspace={deleteWorkspace}
      />

      {/* The agent/session list only applies to agent-scoped views. Board and
          schedules are workspace-scoped, so the list is hidden there. */}
      {/* Chat: a sessions-only list (agents now live in their own view). */}
      {view === 'chat' && (
        <SessionsSidebar
          sessions={chatSessions}
          agents={agents}
          activeSessionId={activeSessionId}
          streamingSessionIds={chat.streamingSessions}
          newDisabled={agents.length === 0}
          onSelectSession={selectSession}
          onNewSession={newSession}
          onRefresh={refreshSessions}
          onRenameSession={renameSession}
          onGenerateTitle={regenerateSessionTitle}
          onCopyPath={copySessionPath}
          onRevealFolder={revealSession}
          onDeleteSession={deleteSession}
          onSetArchived={setSessionArchived}
          onSetPinned={setSessionPinned}
        />
      )}

      {/* Agent-scoped views need an agent picker; reuse the roster as a sidebar. */}
      {view === 'memory' && (
        <AgentRoster
          agents={agents}
          defaultAgentId={defaultAgentId}
          onSelectAgent={pickAgent}
          onCreateAgent={createAgent}
          onUpdateAgent={updateAgent}
        />
      )}

      {/* Key the view subtree by the active workspace so switching (or creating
          and switching into) a workspace REMOUNTS every panel. The panels here
          fetch their own workspace-scoped data on mount (flows, tasks, schedules,
          executions, network, artifacts, secrets, skills, market, budget, logs,
          memory), so without a remount they would keep showing the previous
          workspace's data until a manual page refresh. App-level agents/sessions
          are reset+refetched by the activeWorkspaceId effect above. */}
      <main key={activeWorkspaceId ?? 'none'} className="flex h-full min-w-0 flex-1 flex-col">
        <header className="flex items-center justify-between border-b border-[var(--color-border)] px-6 py-3">
          <div className="flex items-center gap-2">
            <span className="text-sm font-semibold">{VIEW_TITLE[view]}</span>
            {view === 'chat' && (
              <span className="text-sm text-[var(--color-text-dim)]">
                · {agents.find((a) => a.id === activeAgentId)?.name ?? 'Ajan seçilmedi'}
              </span>
            )}
          </div>
          <div className="flex items-center gap-3">
            {view === 'chat' && (
              <ChatMeters
                agentId={activeAgentId}
                sessionId={activeSessionId}
                refreshKey={meterRefresh}
                onError={setError}
                onOpenBudget={() => setView('budget')}
              />
            )}
            {view === 'chat' && activeSessionId && (
              <div className="flex items-center gap-1.5">
                {/* Folder shortcuts (moved here from the detail panel's Klasör card). */}
                <button
                  onClick={copyActiveSessionPath}
                  title="Oturum klasörü yolunu kopyala"
                  className="flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
                >
                  {pathCopied ? <Check size={15} className="shrink-0" /> : <ClipboardCopy size={15} className="shrink-0" />}
                  <span className="hidden sm:inline">{pathCopied ? 'Kopyalandı' : 'Yolu kopyala'}</span>
                </button>
                <button
                  onClick={() => revealSession(activeSessionId)}
                  title="Oturum klasörünü aç"
                  className="flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
                >
                  <FolderOpen size={15} className="shrink-0" />
                  <span className="hidden sm:inline">Aç</span>
                </button>
                <button
                  onClick={toggleDetail}
                  title="Oturum bilgisi panelini aç/kapat"
                  className={`flex items-center gap-1.5 rounded-lg border px-2.5 py-1 text-xs transition ${
                    detailOpen
                      ? 'border-[var(--color-accent)] text-[var(--color-accent)]'
                      : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:text-[var(--color-accent)]'
                  }`}
                >
                  <PanelRight size={15} className="shrink-0" />
                  <span className="hidden sm:inline">Detay</span>
                </button>
              </div>
            )}
            {error && (
              <span className="rounded bg-[color-mix(in_srgb,var(--color-danger)_15%,transparent)] px-2 py-1 text-xs text-[var(--color-danger)]">
                {error}
              </span>
            )}
          </div>
        </header>

        {view === 'chat' && (
          <>
            <MessageList
              messages={messages}
              sessionId={activeSessionId ?? undefined}
              pending={chat.activePending}
              agents={agents}
              artifacts={sessionArtifacts}
              streaming={chat.activeStreaming}
              highlightMessageId={scrollToMsgId}
              onHighlightConsumed={() => setScrollToMsgId(null)}
              onOpenFile={openFile}
              onOpenArtifact={openArtifact}
              onDeleteMessage={deleteMessage}
              onRetry={chat.retryMessage}
              onFeedback={rateMessage}
              onOpenAgent={openAgentSettings}
            />
            {chat.activeAsk &&
              (chat.activeAsk.kind === 'permission' ? (
                <PermissionPrompt ask={chat.activeAsk} onAnswer={chat.answerAsk} />
              ) : chat.activeAsk.kind === 'plan' ? (
                <PlanPrompt ask={chat.activeAsk} onAnswer={chat.answerAsk} />
              ) : (
                <AskPrompt ask={chat.activeAsk} onAnswer={chat.answerAsk} />
              ))}
            <TodoPanel todos={currentTodos} />
            <PendingTray items={chat.activeQueued} onRemove={chat.removePending} />
            {chat.activeWakeWait && (
              <WakeWaitBanner
                reason={chat.activeWakeWait.reason}
                fireAt={chat.activeWakeWait.fireAt}
                onCancel={chat.cancelWake}
              />
            )}
            <Composer
              disabled={!activeSessionId}
              sessionId={activeSessionId ?? undefined}
              streaming={chat.activeStreaming}
              waiting={!!chat.activeWakeWait}
              onCancelWait={chat.cancelWake}
              onSend={(text, attachments) => chat.sendMessage(text, undefined, attachments)}
              onStop={chat.stopTurn}
              onInterrupt={chat.interruptTurn}
              onQueue={chat.queueMessage}
              onSteer={chat.steerTurn}
              thinkingLevel={chat.thinkingLevel}
              onThinkingLevelChange={chat.setThinkingLevel}
              permissionMode={chat.permissionMode}
              onPermissionModeChange={chat.setPermissionMode}
              agentId={activeAgentId ?? ''}
              onAgentChange={changeChatAgent}
              agents={agents}
              commands={chat.chatCommands}
              artifacts={sessionArtifacts}
            />
          </>
        )}
        {view === 'agents' && (
          <AgentsView
            agents={agents}
            defaultAgentId={defaultAgentId}
            selectedId={activeAgentId}
            onSelectAgent={focusAgent}
            onSetDefault={pickAgent}
            onCreateAgent={createAgent}
            onUpdateAgent={updateAgent}
            onDeleteAgent={deleteAgent}
            onRefresh={() =>
              api
                .listAgents()
                .then(setAgents)
                .catch((e) => setError((e as Error).message))
            }
            onError={setError}
            onOpenExecution={openExecution}
          />
        )}
        {view === 'executions' && (
          <ExecutionsPanel
            agents={agents}
            onError={setError}
            onOpenFile={openFile}
            onOpenArtifact={openArtifact}
            onOpenFlowRun={openFlowRun}
            focusId={executionTarget}
            onSelectExecution={setExecutionTarget}
          />
        )}
        {view === 'network' && (
          <Suspense
            fallback={
              <div className="flex-1 p-6 text-sm text-[var(--color-text-dim)]">Ağ yükleniyor…</div>
            }
          >
            <NetworkPanel onError={setError} />
          </Suspense>
        )}
        {view === 'board' && <TaskBoard agents={agents} onError={setError} />}
        {view === 'schedules' && (
          <Schedules agents={agents} focusId={scheduleTarget} onError={setError} />
        )}
        {view === 'memory' && (
          <MemoryPanel
            agent={agents.find((a) => a.id === activeAgentId) ?? null}
            onError={setError}
          />
        )}
        {view === 'flows' && (
          <Suspense
            fallback={
              <div className="flex-1 p-6 text-sm text-[var(--color-text-dim)]">Akışlar yükleniyor…</div>
            }
          >
            <FlowsPanel agents={agents} onError={setError} openFlowId={flowTarget} />
          </Suspense>
        )}
        {view === 'artifacts' && (
          <ArtifactsPanel
            onError={setError}
            agents={agents}
            selectedId={artifactTarget}
            onOpenSession={(sid) => {
              setView('chat')
              selectSession(sid)
            }}
          />
        )}
        {view === 'skills' && <SkillsPanel onError={setError} />}
        {view === 'market' && (
          <MarketPanel
            onError={setError}
            onManageSecrets={openSecrets}
            onInstalled={(kind) => {
              // Refresh the App-level agents list so a freshly installed agent
              // shows on the Agents screen without a manual reload. Flows/skills/
              // providers panels reload on their own mount.
              if (kind === 'agent') api.listAgents().then(setAgents).catch(() => {})
            }}
          />
        )}
        {view === 'budget' && <BudgetPanel onError={setError} />}
        {view === 'logs' && <LogsPanel onError={setError} />}
        {view === 'workspace' && (
          <WorkspaceView
            onError={setError}
            onWorkspaceChanged={refreshWorkspaces}
            onDeleteWorkspace={deleteActiveWorkspace}
            onAppearanceSaved={onAppearanceSaved}
            tab={workspaceTab}
            onTabChange={setWorkspaceTab}
          />
        )}
        {view === 'settings' && (
          <SettingsPanel
            onError={setError}
            onSaved={applyClientPrefs}
            commands={chat.chatCommands}
            cat={settingsCat}
            onCatChange={setSettingsCat}
            reloadNonce={settingsNonce}
          />
        )}
      </main>

      {view === 'chat' && detailOpen && activeSessionId && (
        <SessionDetailPanel
          sessionId={activeSessionId}
          refreshKey={meterRefresh}
          onClose={toggleDetail}
          onError={setError}
          onGenerateTitle={regenerateSessionTitle}
          onRename={renameSession}
          onDeleteSession={deleteSession}
          onSelectSession={selectSession}
        />
      )}

      {/* Artifact quick-preview overlay: opened by clicking an artifact card/chip
          in chat or the activity feed. Independent of the current view. */}
      {previewArtifactId && (
        <ArtifactPreviewModal
          artifactId={previewArtifactId}
          onClose={() => setPreviewArtifactId(null)}
          onOpenFull={openArtifactFull}
          onError={setError}
        />
      )}
    </div>
  )
}
