// useSessionsController owns the workspace-scoped chat data model: the agent
// roster, the session list, the open transcript, and every action that mutates
// them (select/create/rename/archive/pin/delete, per-message actions, agent
// CRUD). App.tsx composes this with the chat-stream hook and the layout.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from '@/api'
import type { Agent, AgentPatch, Artifact, Message, Session } from '@/types'
import type { useChatStream } from '@/features/chat/useChatStream'
import { copyToClipboard } from '@/shared/lib/clipboard'
import type { View } from './NavRail'
import type { Route } from './url'
import { INITIAL_ROUTE } from './useAppNavigation'
import { isChatKind } from './viewRegistry'

export interface SessionsControllerParams {
  activeWorkspaceId: string | null
  setError: (msg: string | null) => void
  setView: (v: View) => void
  // Deep-link hand-off: the workspace-load effect passes a routed execution id
  // to the Activity screen (it loads its own list; we just select the run).
  setExecutionTarget: (id: string | null) => void
}

export function useSessionsController({
  activeWorkspaceId,
  setError,
  setView,
  setExecutionTarget,
}: SessionsControllerParams) {
  const [agents, setAgents] = useState<Agent[]>([])
  const [sessions, setSessions] = useState<Session[]>([])
  const [messages, setMessages] = useState<Message[]>([])
  const [activeAgentId, setActiveAgentId] = useState<string | null>(null)
  const [activeSessionId, setActiveSessionId] = useState<string | null>(null)
  // True from the moment a workspace becomes active until its agents+sessions
  // have landed. While it holds, the chat screen shows a skeleton instead of the
  // "start a new chat" empty state, which would otherwise flash for a returning
  // user whose session list simply hasn't arrived yet.
  const [bootstrapping, setBootstrapping] = useState(true)
  // True while the open session's transcript is being fetched.
  const [messagesLoading, setMessagesLoading] = useState(false)
  // Bumped to remount the Composer so it re-reads its persisted draft — used to
  // restore a rewound prompt back into the input box.
  const [composerKey, setComposerKey] = useState(0)
  // Id of a freshly-created chat that should receive input focus. Set only when
  // the user opens a NEW chat (newSession); switching to an existing session
  // leaves it unchanged so the composer does NOT steal focus on plain selection.
  const [focusSessionId, setFocusSessionId] = useState<string | null>(null)
  const [meterRefresh, setMeterRefresh] = useState(0)
  const bumpMeter = useCallback(() => setMeterRefresh((n) => n + 1), [])
  // Entity selection carried by an initial/cross-workspace deep link, consumed
  // once by the workspace-load effect after agents+sessions arrive.
  const pendingRouteRef = useRef<Route | null>(INITIAL_ROUTE)
  // Default agent for NEW sessions (chosen from the roster). Persisted so it
  // survives reloads; unmentioned turns in a session use the session's own agent.
  const [defaultAgentId, setDefaultAgentId] = useState<string | null>(
    () => localStorage.getItem('tionswarm.defaultAgentId'),
  )
  // Live handle to the chat hook for effects declared ABOVE its definition (the
  // messages-load effect): the ref is read post-render when the binding is set,
  // sidestepping the temporal-dead-zone the const would hit in a deps array.
  const chatRef = useRef<ReturnType<typeof useChatStream> | null>(null)

  // Mirror the active session id into a ref so once-mounted handlers can tell
  // whether an incoming chat completion belongs to the open transcript.
  const activeSessionIdRef = useRef<string | null>(null)
  useEffect(() => {
    activeSessionIdRef.current = activeSessionId
  }, [activeSessionId])

  // Mirror the open transcript into a ref so the chat hook's retry can read the
  // current messages (to find the user prompt behind a failed turn) without
  // re-binding its callbacks on every message update.
  const messagesRef = useRef<Message[]>(messages)
  messagesRef.current = messages

  // Load agents + ALL sessions whenever the active workspace changes (the chat
  // is session-based: sessions are listed flat, not nested under an agent).
  useEffect(() => {
    if (!activeWorkspaceId) return
    setAgents([])
    setSessions([])
    setMessages([])
    setActiveAgentId(null)
    setActiveSessionId(null)
    setBootstrapping(true)
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
            want.view === 'agents' &&
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
      .finally(() => {
        if (!cancelled) setBootstrapping(false)
      })
    return () => {
      cancelled = true
    }
  }, [activeWorkspaceId, setError, setExecutionTarget])

  // Keep the default agent (for new sessions) valid: fall back to the first
  // agent when unset or pointing at a removed agent.
  useEffect(() => {
    if (agents.length === 0) return
    if (!defaultAgentId || !agents.some((a) => a.id === defaultAgentId)) {
      setDefaultAgentId(agents[0].id)
    }
  }, [agents, defaultAgentId])

  // Bumped on every transcript load so only the newest one is allowed to commit:
  // a fast A → B → A switch would otherwise let B's late response overwrite A's
  // transcript, leaving the screen showing the wrong conversation.
  const msgSeqRef = useRef(0)

  // When the active session changes, load its messages. The previous transcript
  // is cleared up-front (rather than lingering until the fetch resolves) and the
  // view shows a skeleton while `messagesLoading` holds. If a turn is still
  // streaming (a mid-turn reload, or switching to a running session), restore the
  // in-progress assistant bubble (agent + steps-so-far) from the inflight snapshot
  // so it isn't blank until the turn ends; the session-step bus then grows it live.
  useEffect(() => {
    const seq = ++msgSeqRef.current
    // Clearing up-front is the point: the outgoing session's transcript must not
    // linger on screen while the next one loads. This is a deliberate cascading
    // render, so the rule is disabled here rather than worked around.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setMessages([])
    if (!activeSessionId) {
      setMessagesLoading(false)
      return
    }
    const sid = activeSessionId
    setMessagesLoading(true)
    api
      .listMessages(sid)
      .then((msgs) => {
        if (msgSeqRef.current !== seq) return
        setMessages(msgs)
        // chat is bound post-render by App (chatRef.current = chat), so the
        // binding is initialised by the time this callback fires (same pattern
        // as the SSE onEvent handler). Kept out of the deps array via the ref.
        // recoverInflight restores the NON-owning path (server snapshot + bus);
        // reseedLive restores the bubble THIS window is actively streaming (which
        // listMessages above just wiped). The two are mutually exclusive per session.
        // Both run AFTER the commit above so the live bubble is not wiped by it.
        void chatRef.current?.recoverInflight(sid, msgs)
        chatRef.current?.reseedLive(sid, msgs)
      })
      .catch((e) => {
        if (msgSeqRef.current !== seq) return
        setError((e as Error).message)
      })
      .finally(() => {
        if (msgSeqRef.current !== seq) return
        setMessagesLoading(false)
      })
  }, [activeSessionId, setError])

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
  }, [setError])

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
  }, [setError])

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
    [activeSessionId, setError],
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
  }, [refreshSessions, setError])

  const copySessionPath = useCallback(async (id: string) => {
    try {
      const { path } = await api.sessionPath(id)
      await copyToClipboard(path, 'Yolu kopyalayın (Ctrl+C, Enter):')
    } catch (e) {
      setError((e as Error).message)
    }
  }, [setError])

  const revealSession = useCallback(async (id: string) => {
    try {
      await api.revealSession(id)
    } catch (e) {
      setError((e as Error).message)
    }
  }, [setError])

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
    [activeSessionId, setError],
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
    [refreshSessions, setError],
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
  }, [setError])

  // Pick the default agent for NEW sessions (from the roster).
  const pickDefaultAgent = useCallback((id: string) => {
    setDefaultAgentId(id)
    localStorage.setItem('tionswarm.defaultAgentId', id)
  }, [])

  // Roster click: set it as the default agent (for new chats) and as the active
  // agent (so the Tools panel, which is agent-scoped, follows along).
  const pickAgent = useCallback(
    (id: string) => {
      setActiveAgentId(id)
      pickDefaultAgent(id)
    },
    [pickDefaultAgent],
  )

  // Focus an agent across agent-scoped views (Agents/Tools) without
  // changing which agent is the default for new chats.
  const focusAgent = useCallback((id: string) => {
    setActiveAgentId(id)
  }, [])

  // Open an agent's settings page (Agents view, that agent selected). Used by the
  // chat transcript so clicking an assistant's avatar/name jumps to its settings.
  const openAgentSettings = useCallback((id: string) => {
    setActiveAgentId(id)
    setView('agents')
  }, [setView])

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
    [pickDefaultAgent, setError],
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
  }, [setError])

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
    // Mark this new chat as the one that should auto-focus the input (the composer
    // focuses only when the active session matches this id).
    setFocusSessionId(s.id)
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
  }, [setError])

  // Manual chats for the chat sidebar (other kinds live in the Activity view).
  const chatSessions = useMemo(() => sessions.filter((s) => isChatKind(s.kind)), [sessions])

  return {
    // state
    agents, setAgents,
    sessions, setSessions,
    messages, setMessages,
    activeAgentId, activeSessionId,
    bootstrapping, messagesLoading,
    chatSessions,
    sessionArtifacts,
    defaultAgentId,
    composerKey, setComposerKey,
    focusSessionId, setFocusSessionId,
    scrollToMsgId, setScrollToMsgId,
    meterRefresh, setMeterRefresh, bumpMeter,
    // refs
    activeSessionIdRef, messagesRef, chatRef, pendingRouteRef,
    // actions
    refreshSessions, changeChatAgent, selectSession, renameSession,
    setSessionArchived, setSessionPinned, copySessionPath, revealSession,
    deleteSession, deleteMessage, rateMessage, pickDefaultAgent, pickAgent,
    focusAgent, openAgentSettings, createAgent, updateAgent, deleteAgent,
    newSession, regenerateSessionTitle,
  }
}
