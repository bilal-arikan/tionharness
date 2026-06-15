import { useEffect, useState, useCallback, useRef } from 'react'
import { api, setActiveWorkspace, getActiveWorkspace } from './api'
import type { Agent, AgentPatch, Session, Message, Workspace, AppSettings } from './types'
import { NavRail, type View } from './components/NavRail'
import { Sidebar } from './components/Sidebar'
import { MessageList } from './components/MessageList'
import { Composer } from './components/Composer'
import { TaskBoard } from './components/TaskBoard'
import { Schedules } from './components/Schedules'
import { MemoryPanel } from './components/MemoryPanel'
import { ToolsPanel } from './components/ToolsPanel'
import { FlowsPanel } from './components/FlowsPanel'
import { ChatMeters } from './components/ChatMeters'
import { SettingsPanel } from './components/SettingsPanel'
import { LogsPanel } from './components/LogsPanel'
import { isImagePath, mediaUrl } from './lib/paths'
import { applyTheme } from './lib/theme'
import { applyKeepAwake, ensureNotificationPermission, notify } from './lib/clientPrefs'

const VIEW_TITLE: Record<View, string> = {
  chat: 'Sohbet',
  board: 'Görevler',
  schedules: 'Zamanlamalar',
  memory: 'Hafıza',
  tools: 'Araçlar',
  flows: 'Akışlar',
  logs: 'Loglar',
  settings: 'Ayarlar',
}

export default function App() {
  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  const [activeWorkspaceId, setActiveWorkspaceId] = useState<string | null>(
    getActiveWorkspace(),
  )
  const [agents, setAgents] = useState<Agent[]>([])
  const [sessions, setSessions] = useState<Session[]>([])
  const [messages, setMessages] = useState<Message[]>([])
  const [activeAgentId, setActiveAgentId] = useState<string | null>(null)
  const [activeSessionId, setActiveSessionId] = useState<string | null>(null)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [view, setView] = useState<View>('chat')
  const [meterRefresh, setMeterRefresh] = useState(0)
  // Desktop-notification preference, read live in sendMessage without re-binding.
  const notifyEnabled = useRef(false)

  // Apply the client-side preferences carried by app settings.
  const applyClientPrefs = useCallback((s: { theme: AppSettings['theme']; accent: string; keepAwake: boolean; desktopNotifications: boolean }) => {
    applyTheme(s.theme, s.accent)
    applyKeepAwake(s.keepAwake)
    ensureNotificationPermission(s.desktopNotifications)
    notifyEnabled.current = s.desktopNotifications
  }, [])

  // Load global settings once and apply theme + client-side behaviours.
  useEffect(() => {
    api.getSettings().then(applyClientPrefs).catch((e) => setError(e.message))
  }, [applyClientPrefs])

  // Initial load: workspaces. Pick active (saved or first).
  useEffect(() => {
    api
      .listWorkspaces()
      .then((list) => {
        setWorkspaces(list)
        const saved = getActiveWorkspace()
        const valid = list.find((w) => w.id === saved)
        const chosen = valid?.id ?? list[0]?.id ?? null
        if (chosen) {
          setActiveWorkspace(chosen)
          setActiveWorkspaceId(chosen)
        }
      })
      .catch((e) => setError(e.message))
  }, [])

  // Load agents whenever the active workspace changes (full reset).
  useEffect(() => {
    if (!activeWorkspaceId) return
    setAgents([])
    setSessions([])
    setMessages([])
    setActiveAgentId(null)
    setActiveSessionId(null)
    api.listAgents().then(setAgents).catch((e) => setError(e.message))
  }, [activeWorkspaceId])

  // Clicking a file path: open images inline (new tab via the file server),
  // copy other paths to the clipboard as a best-effort action.
  const openFile = useCallback((path: string) => {
    if (isImagePath(path)) {
      window.open(mediaUrl(path), '_blank')
    } else {
      navigator.clipboard?.writeText(path).catch(() => {})
    }
  }, [])

  const switchWorkspace = useCallback((id: string) => {
    setActiveWorkspace(id)
    setActiveWorkspaceId(id)
  }, [])

  const createWorkspace = useCallback(async (name: string) => {
    try {
      const wsNew = await api.createWorkspace(name)
      setWorkspaces((prev) => [...prev, wsNew])
      setActiveWorkspace(wsNew.id)
      setActiveWorkspaceId(wsNew.id)
    } catch (e) {
      setError((e as Error).message)
    }
  }, [])

  // When the active agent changes, load its sessions.
  useEffect(() => {
    if (!activeAgentId) return
    api.listSessions(activeAgentId).then((s) => {
      setSessions(s)
      setActiveSessionId(s.length > 0 ? s[0].id : null)
    })
  }, [activeAgentId])

  // When the active session changes, load its messages.
  useEffect(() => {
    if (!activeSessionId) {
      setMessages([])
      return
    }
    api.listMessages(activeSessionId).then(setMessages)
  }, [activeSessionId])

  const createAgent = useCallback(
    async (name: string, soul: string, provider: string) => {
      try {
        const agent = await api.createAgent({ name, soul, provider })
        setAgents((prev) => [agent, ...prev])
        setActiveAgentId(agent.id)
      } catch (e) {
        setError((e as Error).message)
      }
    },
    [],
  )

  const updateAgent = useCallback(async (id: string, patch: AgentPatch) => {
    const updated = await api.updateAgent(id, patch)
    setAgents((prev) => prev.map((a) => (a.id === id ? updated : a)))
  }, [])

  const newSession = useCallback(async () => {
    if (!activeAgentId) return
    const s = await api.createSession(activeAgentId)
    setSessions((prev) => [s, ...prev])
    setActiveSessionId(s.id)
    setMessages([])
  }, [activeAgentId])

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

  const sendMessage = useCallback(
    async (text: string) => {
      if (!activeSessionId) return
      setError(null)

      // Optimistic user bubble.
      const optimistic: Message = {
        id: `tmp-${Date.now()}`,
        sessionId: activeSessionId,
        role: 'user',
        text,
        createdAt: Math.floor(Date.now() / 1000),
      }
      setMessages((prev) => [...prev, optimistic])
      setPending(true)

      try {
        const res = await api.chat(activeSessionId, text)
        // Replace optimistic with canonical pair from server.
        setMessages((prev) => [
          ...prev.filter((m) => m.id !== optimistic.id),
          res.userMessage,
          res.replyMessage,
        ])
        setSessions((prev) =>
          prev.map((s) =>
            s.id === activeSessionId
              ? {
                  ...s,
                  messageCount: s.messageCount + 2,
                  title: res.sessionTitle ?? s.title,
                }
              : s,
          ),
        )
        setMeterRefresh((n) => n + 1)
        notify(notifyEnabled.current, 'SwarmGo — yanıt hazır', res.reply)
      } catch (e) {
        setError((e as Error).message)
        setMessages((prev) => prev.filter((m) => m.id !== optimistic.id))
      } finally {
        setPending(false)
      }
    },
    [activeSessionId],
  )

  return (
    <div className="flex h-full">
      <NavRail
        view={view}
        onSelectView={setView}
        workspaces={workspaces}
        activeWorkspaceId={activeWorkspaceId}
        onSwitchWorkspace={switchWorkspace}
        onCreateWorkspace={createWorkspace}
      />

      {/* The agent/session list only applies to agent-scoped views. Board and
          schedules are workspace-scoped, so the list is hidden there. */}
      {(view === 'chat' || view === 'memory' || view === 'tools') && (
        <Sidebar
          agents={agents}
          sessions={sessions}
          activeAgentId={activeAgentId}
          activeSessionId={activeSessionId}
          onSelectAgent={setActiveAgentId}
          onSelectSession={setActiveSessionId}
          onCreateAgent={createAgent}
          onUpdateAgent={updateAgent}
          onNewSession={newSession}
          onRegenerateSessionTitle={regenerateSessionTitle}
        />
      )}

      <main className="flex h-full flex-1 flex-col">
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
            {error && (
              <span className="rounded bg-red-500/15 px-2 py-1 text-xs text-red-400">
                {error}
              </span>
            )}
          </div>
        </header>

        {view === 'chat' && (
          <>
            <MessageList messages={messages} pending={pending} onOpenFile={openFile} />
            <Composer disabled={!activeSessionId || pending} onSend={sendMessage} />
          </>
        )}
        {view === 'board' && <TaskBoard agents={agents} onError={setError} />}
        {view === 'schedules' && <Schedules agents={agents} onError={setError} />}
        {view === 'memory' && (
          <MemoryPanel
            agent={agents.find((a) => a.id === activeAgentId) ?? null}
            onError={setError}
          />
        )}
        {view === 'tools' && (
          <ToolsPanel
            agent={agents.find((a) => a.id === activeAgentId) ?? null}
            onError={setError}
          />
        )}
        {view === 'flows' && <FlowsPanel agents={agents} onError={setError} />}
        {view === 'logs' && <LogsPanel onError={setError} />}
        {view === 'settings' && (
          <SettingsPanel
            onError={setError}
            onSaved={applyClientPrefs}
            onWorkspaceChanged={() => api.listWorkspaces().then(setWorkspaces).catch(() => {})}
          />
        )}
      </main>
    </div>
  )
}
