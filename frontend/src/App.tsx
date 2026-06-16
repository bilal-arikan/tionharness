import { useEffect, useState, useCallback, useRef } from 'react'
import { api, getActiveWorkspace } from './api'
import type { Agent, AgentPatch, Session, Message, AppSettings, AppEvent } from './types'
import { NavRail, type View } from './components/NavRail'
import { SessionsSidebar } from './components/sessions/SessionsSidebar'
import { AgentRoster } from './components/agents/AgentRoster'
import { AgentsView } from './components/agents/AgentsView'
import { MessageList } from './components/chat/MessageList'
import { Composer } from './components/chat/Composer'
import { AskPrompt } from './components/chat/AskPrompt'
import { PendingTray } from './components/chat/PendingTray'
import { TaskBoard } from './components/panels/TaskBoard'
import { Schedules } from './components/panels/Schedules'
import { MemoryPanel } from './components/panels/MemoryPanel'
import { ToolsPanel } from './components/panels/ToolsPanel'
import { FlowsPanel } from './components/panels/FlowsPanel'
import { ArtifactsPanel } from './components/panels/ArtifactsPanel'
import { ChatMeters } from './components/panels/ChatMeters'
import { SessionDetailPanel } from './components/sessions/SessionDetailPanel'
import { SettingsPanel } from './components/SettingsPanel'
import { LogsPanel } from './components/panels/LogsPanel'
import { useWorkspaces } from './hooks/useWorkspaces'
import { useChatStream } from './hooks/useChatStream'
import { isImagePath, mediaUrl } from './lib/paths'
import { applyTheme } from './lib/theme'
import { applyKeepAwake, ensureNotificationPermission, notify } from './lib/clientPrefs'

const VIEW_TITLE: Record<View, string> = {
  chat: 'Sohbet',
  agents: 'Ajanlar',
  board: 'Görevler',
  schedules: 'Zamanlamalar',
  memory: 'Hafıza',
  tools: 'Araçlar',
  flows: 'Akışlar',
  artifacts: 'Artifactlar',
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
  const [view, setView] = useState<View>('chat')
  const [meterRefresh, setMeterRefresh] = useState(0)
  const bumpMeter = useCallback(() => setMeterRefresh((n) => n + 1), [])

  const {
    workspaces,
    activeWorkspaceId,
    unreadWs,
    setUnreadWs,
    switchWorkspace,
    createWorkspace,
    deleteActiveWorkspace,
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
  const [artifactTarget, setArtifactTarget] = useState<string | null>(null)

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
    api.listAgents().then(setAgents).catch((e) => setError(e.message))
    api
      .listSessions()
      .then((s) => {
        setSessions(s)
        setActiveSessionId(s.length > 0 ? s[0].id : null)
        setActiveAgentId(s.length > 0 ? s[0].agentId : null)
      })
      .catch((e) => setError(e.message))
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

  // Autonomous-event handler: raise a desktop notification whose click deep-links
  // to the event's target (chat session, board, or logs). Refreshed each render
  // so the stable SSE subscription below always sees current closures/state.
  onEventRef.current = (e: AppEvent) => {
    // Badge any non-active workspace that produced activity (incl. completed
    // chats), so the switcher shows where to look.
    if (e.workspaceId && e.workspaceId !== getActiveWorkspace()) {
      setUnreadWs((prev) => {
        if (prev.has(e.workspaceId)) return prev
        const next = new Set(prev)
        next.add(e.workspaceId)
        return next
      })
    }
    // Same-workspace activity (chat/heartbeat/schedule) updates the session list
    // so unread dots, ordering and times stay live without a manual refresh.
    if (!e.workspaceId || e.workspaceId === getActiveWorkspace()) {
      refreshSessions()
      // A chat reply that completed server-side after the SSE stream closed
      // (e.g. the user refreshed mid-turn and the detached turn finished) is not
      // in the open transcript. If it belongs to the session being viewed,
      // reload its messages so the reply appears without a manual reselect.
      const sid = e.target?.sessionId
      if (e.type === 'chat' && sid && sid === activeSessionIdRef.current) {
        api.listMessages(sid).then(setMessages).catch(() => {})
      }
    }
    // Chat completions only drive the badge (the streaming turn already raises
    // its own reply notification); other event types raise a desktop
    // notification that deep-links to the target on click.
    if (e.type === 'chat') return
    notify(notifyEnabled.current, e.title, e.body, () => {
      const t = e.target || {}
      if (e.workspaceId && e.workspaceId !== getActiveWorkspace()) {
        switchWorkspace(e.workspaceId)
      }
      if (t.view) setView(t.view as View)
      if (t.sessionId) selectSession(t.sessionId)
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

  return (
    <div className="flex h-full">
      <NavRail
        view={view}
        onSelectView={setView}
        workspaces={workspaces}
        activeWorkspaceId={activeWorkspaceId}
        unreadWorkspaceIds={unreadWs}
        onSwitchWorkspace={switchWorkspace}
        onCreateWorkspace={createWorkspace}
      />

      {/* The agent/session list only applies to agent-scoped views. Board and
          schedules are workspace-scoped, so the list is hidden there. */}
      {/* Chat: a sessions-only list (agents now live in their own view). */}
      {view === 'chat' && (
        <SessionsSidebar
          sessions={sessions}
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

      {/* Agent-scoped views need an agent picker; reuse the roster as a sidebar. */}
      {(view === 'memory' || view === 'tools') && (
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
              <span className="rounded bg-red-500/15 px-2 py-1 text-xs text-red-400">
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
            />
            {chat.activeAsk && <AskPrompt ask={chat.activeAsk} onAnswer={chat.answerAsk} />}
            <PendingTray items={chat.activeQueued} onRemove={chat.removePending} />
            <Composer
              disabled={!activeSessionId}
              streaming={chat.activeStreaming}
              onSend={chat.sendMessage}
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
            />
          </>
        )}
        {view === 'agents' && (
          <AgentsView
            agents={agents}
            defaultAgentId={defaultAgentId}
            onSetDefault={pickAgent}
            onCreateAgent={createAgent}
            onUpdateAgent={updateAgent}
            onDeleteAgent={deleteAgent}
          />
        )}
        {view === 'board' && <TaskBoard agents={agents} onError={setError} />}
        {view === 'schedules' && <Schedules agents={agents} onError={setError} />}
        {view === 'memory' && (
          <MemoryPanel
            agent={agents.find((a) => a.id === activeAgentId) ?? null}
            onError={setError}
          />
        )}
        {view === 'tools' && <ToolsPanel onError={setError} />}
        {view === 'flows' && <FlowsPanel agents={agents} onError={setError} />}
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
        {view === 'logs' && <LogsPanel onError={setError} />}
        {view === 'settings' && (
          <SettingsPanel
            onError={setError}
            onSaved={applyClientPrefs}
            onWorkspaceChanged={refreshWorkspaces}
            onDeleteWorkspace={deleteActiveWorkspace}
            commands={chat.chatCommands}
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
