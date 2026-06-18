import { useEffect, useState, useCallback, useRef, useMemo, lazy, Suspense } from 'react'
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
import { PendingTray } from './components/chat/PendingTray'
import { TodoPanel } from './components/chat/TodoPanel'
import { latestTodos } from './lib/todos'
import { TaskBoard } from './components/panels/TaskBoard'
import { Schedules } from './components/panels/Schedules'
import { MemoryPanel } from './components/panels/MemoryPanel'
import { ToolsPanel } from './components/panels/ToolsPanel'
// Code-split: the Flows panel pulls in React Flow (~300KB), loaded only when
// the user opens the Akışlar view.
const FlowsPanel = lazy(() =>
  import('./components/panels/FlowsPanel').then((m) => ({ default: m.FlowsPanel })),
)
import { ExecutionsPanel } from './components/panels/ExecutionsPanel'
import { ArtifactsPanel } from './components/panels/ArtifactsPanel'
import { SecretsPanel } from './components/panels/SecretsPanel'
import { SkillsPanel } from './components/panels/SkillsPanel'
import { BudgetPanel } from './components/panels/BudgetPanel'
import { ChatMeters } from './components/panels/ChatMeters'
import { SessionDetailPanel } from './components/sessions/SessionDetailPanel'
import { SettingsPanel } from './components/SettingsPanel'
import { LogsPanel } from './components/panels/LogsPanel'
import { isTypeEnabled } from './lib/notifyPrefs'
import { useWorkspaces } from './hooks/useWorkspaces'
import { useActivity } from './hooks/useActivity'
import { useChatStream } from './hooks/useChatStream'
import { useUrlSync } from './hooks/useUrlSync'
import { parseRoute, routeIdForView, routeFromEvent, buildRoute, type Route } from './lib/url'
import { isImagePath, mediaUrl } from './lib/paths'
import { applyTheme } from './lib/theme'
import { applyKeepAwake, ensureNotificationPermission, notify } from './lib/clientPrefs'

// isChatKind reports whether a session is a manual chat (shown in the chat
// sidebar). Task/flow/schedule/heartbeat transcripts are surfaced in the
// Activity (executions) view instead, so they don't clutter the chat list.
function isChatKind(kind: string): boolean {
  return kind === '' || kind === 'chat'
}

// Parse the deep-link once at module load. If it names a workspace, apply it to
// the api client immediately so useWorkspaces initialises on the routed
// workspace (an unknown id is validated away to the first workspace there).
const INITIAL_ROUTE: Route = parseRoute(window.location.hash)
if (INITIAL_ROUTE.workspaceId) setActiveWorkspace(INITIAL_ROUTE.workspaceId)

const VIEW_TITLE: Record<View, string> = {
  chat: 'Sohbet',
  executions: 'Aktivite',
  agents: 'Ajanlar',
  board: 'Görevler',
  schedules: 'Zamanlamalar',
  memory: 'Hafıza',
  tools: 'Araçlar',
  flows: 'Akışlar',
  artifacts: 'Artifactlar',
  secrets: 'Sırlar',
  skills: 'Beceriler',
  budget: 'Bütçe',
  logs: 'Loglar',
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
  // Entity selection carried by an initial/cross-workspace deep link, consumed
  // once by the workspace-load effect after agents+sessions arrive.
  const pendingRouteRef = useRef<Route | null>(INITIAL_ROUTE)
  const bumpMeter = useCallback(() => setMeterRefresh((n) => n + 1), [])

  const {
    workspaces,
    activeWorkspaceId,
    unreadWs,
    markWorkspaceUnread,
    markWorkspaceRead,
    switchWorkspace,
    createWorkspace,
    deleteActiveWorkspace,
    deleteWorkspace,
    refreshWorkspaces,
  } = useWorkspaces(setError)

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

  // Apply the client-side preferences carried by app settings.
  const applyClientPrefs = useCallback((s: { theme: AppSettings['theme']; accent: string; themePreset?: string; keepAwake: boolean; desktopNotifications: boolean }) => {
    applyTheme(s.theme, s.accent, s.themePreset)
    applyKeepAwake(s.keepAwake)
    ensureNotificationPermission(s.desktopNotifications)
    notifyEnabled.current = s.desktopNotifications
  }, [])

  // Load global settings once and apply theme + client-side behaviours.
  useEffect(() => {
    api.getSettings().then(applyClientPrefs).catch((e) => setError(e.message))
  }, [applyClientPrefs])

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

  // Clicking an artifact card in chat: open the artifacts screen on that one.
  const openArtifact = useCallback((id: string) => {
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

  // Artifacts of the active session — offered by the composer's "#" picker so the
  // user can include an artifact's content in the next turn. Refreshed after each
  // turn (meterRefresh) since a turn may have created new artifacts.
  const [sessionArtifacts, setSessionArtifacts] = useState<Artifact[]>([])
  useEffect(() => {
    if (!activeSessionId) {
      setSessionArtifacts([])
      return
    }
    api.listArtifacts(activeSessionId).then(setSessionArtifacts).catch(() => {})
  }, [activeSessionId, meterRefresh])

  // Mirror the active session id into a ref so the once-mounted event handler
  // can tell whether an incoming chat completion belongs to the open transcript.
  const activeSessionIdRef = useRef<string | null>(null)
  useEffect(() => {
    activeSessionIdRef.current = activeSessionId
  }, [activeSessionId])

  // Reload the session list (fresh order, updated times, unread flags).
  const refreshSessions = useCallback(() => {
    api.listSessions().then(setSessions).catch(() => {})
  }, [])

  // Select a session: reflect its default agent and clear its unread flag.
  const selectSession = useCallback(
    (id: string) => {
      setActiveSessionId(id)
      const sess = sessions.find((s) => s.id === id)
      if (sess) setActiveAgentId(sess.agentId)
      // Optimistically clear unread, then persist on the backend.
      setSessions((prev) => prev.map((s) => (s.id === id ? { ...s, unread: false } : s)))
      api.markSessionRead(id).catch(() => {})
    },
    [sessions],
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

  const deleteSession = useCallback(
    async (id: string) => {
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

  // Autonomous-event handler: raise a desktop notification whose click deep-links
  // to the event's target (chat session, board, or logs). Refreshed each render
  // so the stable SSE subscription below always sees current closures/state.
  onEventRef.current = (e: AppEvent) => {
    // Badge any non-active workspace that produced activity (incl. completed
    // chats), so the switcher shows where to look. markWorkspaceUnread persists
    // the badge so every other open window picks it up via its storage listener.
    if (e.workspaceId && e.workspaceId !== getActiveWorkspace()) {
      markWorkspaceUnread(e.workspaceId)
    }
    // Same-workspace activity (chat/heartbeat/schedule) updates the session list
    // so unread dots, ordering and times stay live without a manual refresh.
    if (!e.workspaceId || e.workspaceId === getActiveWorkspace()) {
      // This window is live-viewing the workspace → the activity has been seen;
      // clear its badge across all windows (a different window may have set it).
      if (e.workspaceId) markWorkspaceRead(e.workspaceId)
      refreshSessions()
      // A chat reply that completed server-side after the SSE stream closed
      // (e.g. the user refreshed mid-turn and the detached turn finished) is not
      // in the open transcript. If it belongs to the session being viewed,
      // reload its messages so the reply appears without a manual reselect.
      const sid = e.target?.sessionId
      if (e.type === 'chat' && sid) {
        // The turn ended (success or failure) — drop any post-reload "thinking"
        // indicator we restored for it, and refresh the open transcript so the
        // reply (if any) appears without a manual reselect.
        chat.clearPending(sid)
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
    notify(notifyEnabled.current, e.title, e.body, () => {
      // Navigate via the deep-link URL: setting the hash drives the URL→state
      // machinery (useUrlSync → applyRoute), which switches workspace and
      // selects the entity correctly even across workspaces. notify() has
      // already focused the window.
      const r = routeFromEvent(e)
      if (r) window.location.hash = buildRoute(r)
    })
  }

  // Subscribe once to the global autonomous-event feed (heartbeat/task/schedule).
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
    const s = await api.createSession(aid)
    setSessions((prev) => [s, ...prev])
    setActiveSessionId(s.id)
    setActiveAgentId(s.agentId)
    setMessages([])
  }, [defaultAgentId, agents])

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
      } else if (r.view === 'settings') {
        setSettingsCat(r.id)
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
    }),
  }
  useUrlSync(route, !!activeWorkspaceId, applyRoute)

  return (
    <div className="flex h-full">
      <NavRail
        view={view}
        onSelectView={setView}
        workspaces={workspaces}
        activeWorkspaceId={activeWorkspaceId}
        unreadWorkspaceIds={unreadWs}
        busyViews={busyViews}
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
          onRenameSession={renameSession}
          onGenerateTitle={regenerateSessionTitle}
          onCopyPath={copySessionPath}
          onRevealFolder={revealSession}
          onDeleteSession={deleteSession}
        />
      )}

      {/* Agent-scoped views need an agent picker; reuse the roster as a sidebar.
          Tools is workspace-scoped (no agent), so it has its own master-detail
          layout and skips the roster. */}
      {view === 'memory' && (
        <AgentRoster
          agents={agents}
          defaultAgentId={defaultAgentId}
          onSelectAgent={pickAgent}
          onCreateAgent={createAgent}
          onUpdateAgent={updateAgent}
        />
      )}

      <main className="flex h-full min-w-0 flex-1 flex-col">
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
              />
            )}
            {view === 'chat' && activeSessionId && (
              <button
                onClick={toggleDetail}
                title="Oturum bilgisi panelini aç/kapat"
                className={`rounded-lg border px-2 py-1 text-sm transition ${
                  detailOpen
                    ? 'border-[var(--color-accent)] text-[var(--color-accent)]'
                    : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:text-[var(--color-accent)]'
                }`}
              >
                ℹ
              </button>
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
              pending={chat.activePending}
              agents={agents}
              streaming={chat.activeStreaming}
              onOpenFile={openFile}
              onOpenArtifact={openArtifact}
              onDeleteMessage={deleteMessage}
            />
            {chat.activeAsk &&
              (chat.activeAsk.kind === 'permission' ? (
                <PermissionPrompt ask={chat.activeAsk} onAnswer={chat.answerAsk} />
              ) : (
                <AskPrompt ask={chat.activeAsk} onAnswer={chat.answerAsk} />
              ))}
            <TodoPanel todos={currentTodos} />
            <PendingTray items={chat.activeQueued} onRemove={chat.removePending} />
            <Composer
              disabled={!activeSessionId}
              sessionId={activeSessionId ?? undefined}
              streaming={chat.activeStreaming}
              onSend={(text, attachments) => chat.sendMessage(text, undefined, attachments)}
              onStop={chat.stopTurn}
              onInterrupt={chat.interruptTurn}
              onQueue={chat.queueMessage}
              onSteer={chat.steerTurn}
              thinkingLevel={chat.thinkingLevel}
              onThinkingLevelChange={chat.setThinkingLevel}
              permissionMode={chat.permissionMode}
              onPermissionModeChange={chat.setPermissionMode}
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
          />
        )}
        {view === 'executions' && (
          <ExecutionsPanel
            agents={agents}
            onError={setError}
            onOpenFile={openFile}
            onOpenArtifact={openArtifact}
          />
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
        {view === 'tools' && <ToolsPanel onError={setError} />}
        {view === 'flows' && (
          <Suspense
            fallback={
              <div className="flex-1 p-6 text-sm text-[var(--color-text-dim)]">Akışlar yükleniyor…</div>
            }
          >
            <FlowsPanel agents={agents} onError={setError} />
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
        {view === 'secrets' && <SecretsPanel onError={setError} />}
        {view === 'skills' && <SkillsPanel onError={setError} />}
        {view === 'budget' && <BudgetPanel onError={setError} />}
        {view === 'logs' && <LogsPanel onError={setError} />}
        {view === 'settings' && (
          <SettingsPanel
            onError={setError}
            onSaved={applyClientPrefs}
            onWorkspaceChanged={refreshWorkspaces}
            onDeleteWorkspace={deleteActiveWorkspace}
            commands={chat.chatCommands}
            onNavigate={setView}
            cat={settingsCat}
            onCatChange={setSettingsCat}
          />
        )}
      </main>

      {view === 'chat' && detailOpen && activeSessionId && (
        <SessionDetailPanel
          sessionId={activeSessionId}
          refreshKey={meterRefresh}
          onClose={toggleDetail}
          onError={setError}
          onCopyPath={copySessionPath}
          onRevealFolder={revealSession}
          onGenerateTitle={regenerateSessionTitle}
          onSummarize={(_, kind) => chat.summarize(kind as 'memory' | 'board' | 'flows' | 'tools')}
          onDeleteSession={deleteSession}
        />
      )}
    </div>
  )
}
