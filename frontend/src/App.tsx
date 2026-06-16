import { useEffect, useState, useCallback, useMemo, useRef } from 'react'
import { api, setActiveWorkspace, getActiveWorkspace } from './api'
import type { Agent, AgentPatch, Session, Message, Workspace, AppSettings, TurnStep, SlashCommand, AppEvent } from './types'
import { NavRail, type View } from './components/NavRail'
import type { NewWorkspaceData } from './components/WorkspaceCreateModal'
import { SessionsSidebar } from './components/SessionsSidebar'
import { AgentRoster } from './components/AgentRoster'
import { AgentsView } from './components/AgentsView'
import { MessageList } from './components/MessageList'
import { Composer } from './components/Composer'
import { AskPrompt, type PendingAsk } from './components/chat/AskPrompt'
import { TaskBoard } from './components/TaskBoard'
import { Schedules } from './components/Schedules'
import { MemoryPanel } from './components/MemoryPanel'
import { ToolsPanel } from './components/ToolsPanel'
import { FlowsPanel } from './components/FlowsPanel'
import { ArtifactsPanel } from './components/ArtifactsPanel'
import { ChatMeters } from './components/ChatMeters'
import { SettingsPanel } from './components/SettingsPanel'
import { LogsPanel } from './components/LogsPanel'
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
  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  const [activeWorkspaceId, setActiveWorkspaceId] = useState<string | null>(
    getActiveWorkspace(),
  )
  // Workspaces (other than the active one) with pending activity, shown as a
  // badge in the switcher. Populated from the autonomous-event feed.
  const [unreadWs, setUnreadWs] = useState<Set<string>>(() => new Set())
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
  // Latest autonomous-event handler, refreshed each render so the once-mounted
  // SSE subscription always navigates with current state/closures.
  const onEventRef = useRef<(e: AppEvent) => void>(() => {})
  // Streaming-turn control: whether a turn is in flight, its abort handle (stop /
  // interrupt) and run id (steer), plus a message queued to send after it ends.
  const [streaming, setStreaming] = useState(false)
  const abortRef = useRef<AbortController | null>(null)
  const runIdRef = useRef('')
  const queuedRef = useRef('')
  // When the agent calls ask_user, the turn pauses and this holds the question
  // until the user answers (delivered to the still-open stream via chatControl).
  const [pendingAsk, setPendingAsk] = useState<PendingAsk | null>(null)
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

  // Clicking an artifact card in chat: open the artifacts screen on that one.
  const openArtifact = useCallback((id: string) => {
    setArtifactTarget(id)
    setView('artifacts')
  }, [])

  const switchWorkspace = useCallback((id: string) => {
    setActiveWorkspace(id)
    setActiveWorkspaceId(id)
    // Switching to a workspace clears its pending-activity badge.
    setUnreadWs((prev) => {
      if (!prev.has(id)) return prev
      const next = new Set(prev)
      next.delete(id)
      return next
    })
  }, [])

  const createWorkspace = useCallback(async (data: NewWorkspaceData) => {
    try {
      const wsNew = await api.createWorkspace(data)
      // Re-fetch the list so the icon/color (stored in ws-settings, absent from
      // the create response) are reflected immediately; fall back to appending.
      try {
        setWorkspaces(await api.listWorkspaces())
      } catch {
        setWorkspaces((prev) => [...prev, wsNew])
      }
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
    [],
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
            // Interactive prompt: the agent paused on ask_user. Surface the
            // question (transient — not added to the persisted trace); the user's
            // answer resumes the turn over the same stream.
            if (st.kind === 'ask') {
              setPendingAsk({ question: st.text || '', options: st.options })
              return
            }
            // Streaming providers emit incremental "delta" steps: append the
            // chunk to the live bubble's text instead of the activity trace.
            if (st.kind === 'delta') {
              const chunk = st.text || ''
              setMessages((prev) =>
                prev.map((x) => (x.id === id ? { ...x, text: x.text + chunk } : x)),
              )
              return
            }
            // Tombstone: retract a previously emitted live step by id.
            if (st.kind === 'tombstone') {
              liveSteps = liveSteps.filter((s) => s.id !== st.ref)
            } else if (st.kind === 'tool_delta' && st.id) {
              // Merge streaming tool output into the existing chunk of the same id.
              const idx = liveSteps.findIndex((s) => s.kind === 'tool_delta' && s.id === st.id)
              if (idx >= 0) {
                const merged = { ...liveSteps[idx], output: (liveSteps[idx].output || '') + (st.output || '') }
                liveSteps = liveSteps.map((s, k) => (k === idx ? merged : s))
              } else {
                liveSteps = [...liveSteps, st]
              }
            } else {
              liveSteps = [...liveSteps, st]
            }
            const json = JSON.stringify(liveSteps)
            setMessages((prev) =>
              prev.map((x) => (x.id === id ? { ...x, steps: json } : x)),
            )
          },
          onReply: (r) => {
            const id = liveId
            setPendingAsk(null)
            setMessages((prev) => prev.map((x) => (x.id === id ? r.replyMessage : x)))
            // Clicking the notification jumps to the source chat session.
            notify(notifyEnabled.current, 'SwarmGo — yanıt hazır', r.replyMessage.text, () => {
              setView('chat')
              selectSession(sid)
            })
          },
          onDone: (d) => {
            void d
            setMeterRefresh((n) => n + 1)
            // The active session got an agent reply (marked unread on the
            // backend); the user is viewing it, so clear that, then reload the
            // list to refresh order/times/counts.
            api.markSessionRead(sid).catch(() => {}).finally(refreshSessions)
          },
          onError: (err) => {
            setError(err)
            setPendingAsk(null)
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
    setPendingAsk(null)
  }, [])

  // Answer: deliver the user's reply to a turn paused on ask_user, resuming it.
  const answerAsk = useCallback((text: string) => {
    setPendingAsk(null)
    if (runIdRef.current) {
      api.chatControl(runIdRef.current, 'answer', text).catch((e) =>
        setError((e as Error).message),
      )
    }
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
              onOpenArtifact={openArtifact}
            />
            {pendingAsk && <AskPrompt ask={pendingAsk} onAnswer={answerAsk} />}
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
        {view === 'tools' && (
          <ToolsPanel
            agent={agents.find((a) => a.id === activeAgentId) ?? null}
            onError={setError}
          />
        )}
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
            onWorkspaceChanged={() => api.listWorkspaces().then(setWorkspaces).catch(() => {})}
            onDeleteWorkspace={deleteActiveWorkspace}
            commands={chatCommands}
          />
        )}
      </main>
    </div>
  )
}
