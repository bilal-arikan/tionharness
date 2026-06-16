import { useEffect, useState, useCallback, useMemo, useRef } from 'react'
import { api, setActiveWorkspace, getActiveWorkspace } from './api'
import type { Agent, AgentPatch, Session, Message, Workspace, AppSettings, TurnStep, SlashCommand } from './types'
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
  // Default agent for NEW sessions (chosen from the roster). Persisted so it
  // survives reloads; unmentioned turns in a session use the session's own agent.
  const [defaultAgentId, setDefaultAgentId] = useState<string | null>(
    () => localStorage.getItem('swarmgo.defaultAgentId'),
  )
  // Desktop-notification preference, read live in sendMessage without re-binding.
  const notifyEnabled = useRef(false)
  // Streaming-turn control: whether a turn is in flight, its abort handle (stop /
  // interrupt) and run id (steer), plus a message queued to send after it ends.
  const [streaming, setStreaming] = useState(false)
  const abortRef = useRef<AbortController | null>(null)
  const runIdRef = useRef('')
  const queuedRef = useRef('')

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

  // Delete the active workspace, then switch to another (backend forbids
  // deleting the last one).
  const deleteActiveWorkspace = useCallback(async () => {
    if (!activeWorkspaceId) return
    const target = workspaces.find((w) => w.id === activeWorkspaceId)
    if (!confirm(`"${target?.name ?? 'Bu workspace'}" ve tüm verisi kalıcı olarak silinsin mi?`)) return
    try {
      await api.deleteWorkspace(activeWorkspaceId)
      const remaining = workspaces.filter((w) => w.id !== activeWorkspaceId)
      setWorkspaces(remaining)
      const next = remaining[0]?.id ?? null
      if (next) {
        setActiveWorkspace(next)
        setActiveWorkspaceId(next)
      }
    } catch (e) {
      setError((e as Error).message)
    }
  }, [activeWorkspaceId, workspaces])

  // When the active session changes, load its messages.
  useEffect(() => {
    if (!activeSessionId) {
      setMessages([])
      return
    }
    api.listMessages(activeSessionId).then(setMessages)
  }, [activeSessionId])

  // Select a session: also reflect its default agent (for the header/meters).
  const selectSession = useCallback(
    (id: string) => {
      setActiveSessionId(id)
      const sess = sessions.find((s) => s.id === id)
      if (sess) setActiveAgentId(sess.agentId)
    },
    [sessions],
  )

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
    [],
  )

  const updateAgent = useCallback(async (id: string, patch: AgentPatch) => {
    const updated = await api.updateAgent(id, patch)
    setAgents((prev) => prev.map((a) => (a.id === id ? updated : a)))
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

  const sendMessage = useCallback(
    async (text: string) => {
      if (!activeSessionId) return
      setError(null)
      const sid = activeSessionId

      // Resolve "@mentions" → ordered agentIds. No mention → the session's
      // default agent answers; multiple → each answers in order.
      const mentioned: string[] = []
      const norm = (v: string) => v.toLowerCase().replace(/\s+/g, '')
      const re = /(?:^|\s)@([^\s@]+)/g
      let mm: RegExpExecArray | null
      while ((mm = re.exec(text)) !== null) {
        const q = norm(mm[1])
        const a =
          agents.find((ag) => norm(ag.name) === q) ??
          agents.find((ag) => norm(ag.name).startsWith(q))
        if (a && !mentioned.includes(a.id)) mentioned.push(a.id)
      }
      const sessAgent = sessions.find((s) => s.id === sid)?.agentId
      const agentIds = mentioned.length ? mentioned : sessAgent ? [sessAgent] : []
      const replyCount = agentIds.length || 1

      const now = Math.floor(Date.now() / 1000)
      const optimistic: Message = {
        id: `tmp-${Date.now()}`,
        sessionId: sid,
        role: 'user',
        text,
        createdAt: now,
      }
      setMessages((prev) => [...prev, optimistic])
      setPending(true)
      const ac = new AbortController()
      abortRef.current = ac
      setStreaming(true)

      // The live bubble for the agent currently answering (multi-agent turns
      // produce several bubbles, one per agent, in order).
      let liveId = ''
      let liveSteps: TurnStep[] = []
      try {
        await api.chatStream(sid, text, agentIds, {
          onMeta: (m) => {
            runIdRef.current = m.runId
            setMessages((prev) =>
              prev.map((x) => (x.id === optimistic.id ? m.userMessage : x)),
            )
          },
          onAgentStart: (a) => {
            liveId = `live-${a.index}-${Date.now()}`
            liveSteps = []
            const bubble: Message = {
              id: liveId,
              sessionId: sid,
              role: 'assistant',
              agentId: a.agentId,
              text: '',
              steps: '[]',
              createdAt: Math.floor(Date.now() / 1000),
            }
            setMessages((prev) => [...prev, bubble])
            setPending(false)
          },
          onStep: (st) => {
            const id = liveId
            // Streaming providers emit incremental "delta" steps: append the
            // chunk to the live bubble's text instead of the activity trace.
            if (st.kind === 'delta') {
              const chunk = st.text || ''
              setMessages((prev) =>
                prev.map((x) => (x.id === id ? { ...x, text: x.text + chunk } : x)),
              )
              return
            }
            liveSteps = [...liveSteps, st]
            const json = JSON.stringify(liveSteps)
            setMessages((prev) =>
              prev.map((x) => (x.id === id ? { ...x, steps: json } : x)),
            )
          },
          onReply: (r) => {
            const id = liveId
            setMessages((prev) => prev.map((x) => (x.id === id ? r.replyMessage : x)))
            // Clicking the notification jumps to the source chat session.
            notify(notifyEnabled.current, 'SwarmGo — yanıt hazır', r.replyMessage.text, () => {
              setView('chat')
              selectSession(sid)
            })
          },
          onDone: (d) => {
            setSessions((prev) =>
              prev.map((s) =>
                s.id === sid
                  ? {
                      ...s,
                      messageCount: s.messageCount + 1 + replyCount,
                      title: d.sessionTitle || s.title,
                    }
                  : s,
              ),
            )
            setMeterRefresh((n) => n + 1)
          },
          onError: (err) => {
            setError(err)
            setMessages((prev) =>
              prev.filter((m) => !m.id.startsWith('live-') && m.id !== optimistic.id),
            )
            // Clicking the notification jumps to the logs view to inspect it.
            notify(notifyEnabled.current, 'SwarmGo — hata', err, () => setView('logs'))
          },
        }, ac.signal)
      } catch (e) {
        // A deliberate stop/interrupt aborts the fetch: keep the partial reply
        // bubble visible and don't surface it as an error.
        if (!ac.signal.aborted) {
          const msg = (e as Error).message
          setError(msg)
          setMessages((prev) =>
            prev.filter((m) => !m.id.startsWith('live-') && m.id !== optimistic.id),
          )
          notify(notifyEnabled.current, 'SwarmGo — hata', msg, () => setView('logs'))
        }
      } finally {
        setPending(false)
        setStreaming(false)
        if (abortRef.current === ac) abortRef.current = null
      }
    },
    [activeSessionId, agents, sessions],
  )

  // After a streaming turn ends, flush a message queued during it.
  useEffect(() => {
    if (!streaming && queuedRef.current) {
      const q = queuedRef.current
      queuedRef.current = ''
      void sendMessage(q)
    }
  }, [streaming, sendMessage])

  // ---- streaming-turn interventions ----

  // Stop: abort the in-flight stream (server cancels via context).
  const stopTurn = useCallback(() => {
    abortRef.current?.abort()
    setStreaming(false)
  }, [])

  // Interrupt: stop the current turn and immediately send a new message.
  const interruptTurn = useCallback(
    (text: string) => {
      abortRef.current?.abort()
      // Let the abort settle before starting the next turn.
      setTimeout(() => void sendMessage(text), 0)
    },
    [sendMessage],
  )

  // Queue: hold a message to auto-send when the current turn finishes.
  const queueMessage = useCallback((text: string) => {
    queuedRef.current = text
  }, [])

  // Steer: deliver live guidance to the running turn (tool loop folds it in).
  const steerTurn = useCallback((text: string) => {
    if (runIdRef.current) {
      api.chatControl(runIdRef.current, 'steer', text).catch((e) => setError((e as Error).message))
    }
  }, [])

  // Slash commands available in the chat composer ("/" menu).
  const chatCommands = useMemo<SlashCommand[]>(
    () => [
      { name: 'new', icon: '➕', description: 'Yeni oturum başlat', run: () => void newSession() },
      {
        name: 'title',
        icon: '⟳',
        description: 'Oturum başlığını yeniden üret',
        run: () => {
          if (activeSessionId) void regenerateSessionTitle(activeSessionId)
        },
      },
      {
        name: 'reflect',
        icon: '✦',
        description: 'Ajana yansıma (dream cycle) ürettir',
        run: () => {
          if (activeAgentId) api.reflect(activeAgentId).catch((e) => setError((e as Error).message))
        },
      },
      { name: 'memory', icon: '⛁', description: 'Hafıza görünümüne geç', run: () => setView('memory') },
      { name: 'tools', icon: '🔌', description: 'Araçlar görünümüne geç', run: () => setView('tools') },
      { name: 'board', icon: '🗂', description: 'Görevler panosuna geç', run: () => setView('board') },
      { name: 'flows', icon: '🔀', description: 'Akışlar görünümüne geç', run: () => setView('flows') },
    ],
    [newSession, regenerateSessionTitle, activeSessionId, activeAgentId],
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
          defaultAgentId={defaultAgentId}
          activeSessionId={activeSessionId}
          onSelectAgent={pickAgent}
          onSelectSession={selectSession}
          onCreateAgent={createAgent}
          onUpdateAgent={updateAgent}
          onNewSession={newSession}
          onRegenerateSessionTitle={regenerateSessionTitle}
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
              pending={pending}
              agents={agents}
              onOpenFile={openFile}
            />
            <Composer
              disabled={!activeSessionId}
              streaming={streaming}
              onSend={sendMessage}
              onStop={stopTurn}
              onInterrupt={interruptTurn}
              onQueue={queueMessage}
              onSteer={steerTurn}
              agents={agents}
              commands={chatCommands}
            />
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
            onDeleteWorkspace={deleteActiveWorkspace}
          />
        )}
      </main>
    </div>
  )
}
