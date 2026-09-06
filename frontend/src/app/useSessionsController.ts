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
import { isWritableSessionKind } from './viewRegistry'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import { SIGNAL_AGENTS } from './eventToRefreshSignals'
import { draftSessionIds, readSessionDraftState } from '@/shared/lib/sessionDrafts'
import { shouldDiscardFreshSession } from './freshSessionCleanup'
import { pickInitialSession } from './pickInitialSession'
import { saveDefaultAgent } from './defaultAgentSave'
import { deleteSessionAndRefresh } from './sessionDelete'
import {
  appendSessionPage,
  chatRouteLookupResolved,
  createSessionListRequestGuard,
  initialSessionLookupIDs,
  mergeSelectedSession,
  sessionListQueryIdentity,
} from './sessionListRequests'

// Sidebar list page size (TSK68 load-more): the session list is fetched one
// page at a time and appended via loadMoreSessions. Kept under the backend's
// maxPageLimit (100) so the server never clamps it silently.
const SESSIONS_PAGE_SIZE = 100
// Trailing settle window for event-driven list refreshes (refreshSessionsSoon).
const SESSIONS_REFRESH_SETTLE_MS = 300
const INITIAL_SESSION_LOOKUP_LIMIT = 50

export interface SessionsControllerParams {
  activeWorkspaceId: string | null
  // The sidebar's chip selection, comma-joined (see useSessionChips). Sent with
  // every list request so the server pages the FILTERED set: paging a mixed list
  // client-side meant a page of 100 could hold three visible chats and the
  // "Daha fazla yükle" button looked broken.
  chipsParam: string
  // The sidebar's free-text title / id search, already debounced by the sidebar.
  // Sent as ?q= so the server filters BEFORE paging: without it a search only
  // saw the rows already loaded and silently missed every older session.
  searchQuery?: string
  setError: (msg: string | null) => void
  setView: (v: View) => void
}

export function useSessionsController({
  activeWorkspaceId,
  chipsParam,
  searchQuery = '',
  setError,
  setView,
}: SessionsControllerParams) {
  // allAgents is what the roster endpoint returns: live agents PLUS the ones
  // marked deleted, which history needs to render a past conversation's author.
  // `agents` is the live subset and stays the default everything else consumes,
  // so no picker, roster or default-agent path can ever offer a deleted agent.
  const [allAgents, setAllAgents] = useState<Agent[]>([])
  const agents = useMemo(() => allAgents.filter((a) => !a.deleted), [allAgents])
  const [sessions, setSessions] = useState<Session[]>([])
  // Paging state for the session list (TSK68 load-more): the sidebar renders
  // the first page and appends with loadMoreSessions. total/hasMore come from
  // the {items,total,offset,limit,hasMore} envelope; a parameter-less call that
  // still returns the legacy array is normalized client-side to the same shape.
  const [sessionsTotal, setSessionsTotal] = useState(0)
  const [sessionsHasMore, setSessionsHasMore] = useState(false)
  // chipKey → count in the request's non-chip scope, straight from the list
  // response. The filtered page cannot answer "how many does this unticked chip
  // hide" by itself, so badges use this map.
  const [sessionChipCounts, setSessionChipCounts] = useState<Record<string, number>>({})
  // The chip selection the in-flight/last request used, kept in a ref so the
  // refresh and load-more callbacks stay stable across chip changes.
  const chipsParamRef = useRef(chipsParam)
  chipsParamRef.current = chipsParam
  // The search the in-flight/last request used; undefined when blank so the
  // request line stays identical to the pre-search one. Synced in an effect
  // (declared before the effect that re-queries on it, so it runs first).
  const searchQueryRef = useRef<string | undefined>(undefined)
  useEffect(() => {
    searchQueryRef.current = searchQuery.trim() || undefined
  }, [searchQuery])
  const activeWorkspaceIdRef = useRef(activeWorkspaceId)
  activeWorkspaceIdRef.current = activeWorkspaceId
  const listQueryIdentityRef = useRef('')
  listQueryIdentityRef.current = sessionListQueryIdentity(activeWorkspaceId, chipsParam)
  const listRequestGuardRef = useRef(createSessionListRequestGuard())
  const listReplacePendingRef = useRef(false)
  const listRefreshQueuedRef = useRef(false)
  const refreshSessionsRef = useRef<(() => Promise<boolean>) | null>(null)
  const runQueuedSessionRefresh = useCallback(() => {
    if (!listRefreshQueuedRef.current) return
    listRefreshQueuedRef.current = false
    void refreshSessionsRef.current?.()
  }, [])
  // How many sessions have been loaded so far — kept in a ref so refreshSessions
  // can refetch the SAME window (instead of collapsing back to one page) without
  // re-creating the callback on every append.
  const sessionsLimitRef = useRef(SESSIONS_PAGE_SIZE)
  const sessionsRef = useRef<Session[]>([])
  // How many rows the server has actually returned for the current chip
  // selection. Distinct from sessions.length, which may also carry the open
  // session that the filter excludes (see withActiveSession).
  const loadedPageSizeRef = useRef(0)
  // Post-commit assignment: only read from callbacks/effects, never during render.
  useEffect(() => {
    sessionsRef.current = sessions
  })
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
  // Default agent for NEW sessions (chosen from the roster). Persisted per-workspace
  // on the backend so switching workspaces does not silently overwrite another
  // workspace's choice. Unmentioned turns in a session use the session's own agent.
  const [defaultAgentId, setDefaultAgentId] = useState<string | null>(null)
  const defaultAgentIdRef = useRef<string | null>(null)
  const defaultAgentSaveInFlightRef = useRef(false)
  const [defaultAgentSaveState, setDefaultAgentSaveState] = useState<'idle' | 'saving' | 'saved'>(
    'idle',
  )
  const setCurrentDefaultAgent = useCallback((id: string | null) => {
    defaultAgentIdRef.current = id
    setDefaultAgentId(id)
  }, [])
  // Whether this workspace's settings were actually READ. False both before the
  // fetch lands and when it fails, and it gates the self-heal write below.
  const [wsSettingsLoaded, setWsSettingsLoaded] = useState(false)
  const agentsTick = useRefreshTrigger(SIGNAL_AGENTS)
  const lastAgentsTickRef = useRef(agentsTick)
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
  // Post-commit assignment: only the chat hook's retry callback reads this.
  useEffect(() => {
    messagesRef.current = messages
  })

  // Keep the OPEN session in the list even when the chip filter excludes it from
  // the server's page. The header, composer gating and transcript all resolve the
  // active session out of this array, so dropping it would blank a conversation
  // the user is reading just because they unticked its chip. The sidebar applies
  // the same chip predicate client-side, so the row still disappears from the
  // list — only the app state keeps it.
  const withActiveSession = useCallback((items: Session[]) => {
    const id = activeSessionIdRef.current
    if (!id || items.some((s) => s.id === id)) return items
    const open = sessionsRef.current.find((s) => s.id === id)
    return mergeSelectedSession(items, open)
  }, [])

  // Load workspace metadata independently from the paged session query. Chip
  // changes only replace the session window and must not reload the roster.
  useEffect(() => {
    if (!activeWorkspaceId) {
      setBootstrapping(false)
      return
    }
    setAllAgents([])
    setSessions([])
    setMessages([])
    setActiveAgentId(null)
    setActiveSessionId(null)
    activeSessionIdRef.current = null
    // The default agent is per-workspace, so the outgoing workspace's pick must
    // not linger while the new one's settings are in flight.
    setCurrentDefaultAgent(null)
    setDefaultAgentSaveState('idle')
    setWsSettingsLoaded(false)
    let cancelled = false
    Promise.all([api.listAgents(), api.getWorkspaceSettings().catch(() => null)])
      .then(([ag, ws]) => {
        if (cancelled) return
        setAllAgents(ag)
        // Restore the per-workspace default agent from the backend. If it points to an
        // agent that no longer exists in this workspace, the self-heal effect below will
        // pick the first agent and persist the correction.
        //
        // settingsLoaded gates that self-heal: when the GET simply FAILED we know
        // nothing about the stored choice, and healing on that would overwrite the
        // user's real pick with agents[0] — turning a transient read error into a
        // permanent write.
        if (ws?.defaultAgentId && ag.some((a) => a.id === ws.defaultAgentId)) {
          setCurrentDefaultAgent(ws.defaultAgentId)
        }
        setWsSettingsLoaded(ws !== null)
      })
      .catch((e) => {
        if (!cancelled) setError((e as Error).message)
      })
    return () => {
      cancelled = true
    }
  }, [activeWorkspaceId, setCurrentDefaultAgent, setError])

  // Load or replace the filtered first page. Deep-link and draft candidates are
  // resolved by one bounded exact-ID query so selection is not limited to page
  // one and does not require loading the whole workspace.
  useEffect(() => {
    if (!activeWorkspaceId) return
    setBootstrapping(true)
    listReplacePendingRef.current = true
    const identity = sessionListQueryIdentity(activeWorkspaceId, chipsParam)
    const token = listRequestGuardRef.current.begin(identity)
    const want = pendingRouteRef.current
    const drafted = draftSessionIds()
    const exactIDs = initialSessionLookupIDs(
      want,
      activeSessionIdRef.current,
      drafted,
      INITIAL_SESSION_LOOKUP_LIMIT,
    )
    const exactLookup = api
      .getSessionsByIds(exactIDs)
      .then((items) => ({ ok: true as const, items }))
      .catch(() => ({ ok: false as const, items: [] as Session[] }))
    const agentLookup =
      want?.view === 'agents'
        ? api
            .listAgents()
            .then((items) => ({ ok: true as const, items }))
            .catch(() => ({ ok: false as const, items: [] as Agent[] }))
        : Promise.resolve({ ok: true as const, items: [] as Agent[] })

    Promise.all([
      api.listSessions({ limit: SESSIONS_PAGE_SIZE, chips: chipsParam, q: searchQueryRef.current }),
      exactLookup,
      agentLookup,
    ])
      .then(([page, exact, routeAgents]) => {
        if (!listRequestGuardRef.current.isCurrent(token, listQueryIdentityRef.current)) return
        const routeResolved = chatRouteLookupResolved(want, page.items, exact.ok)
        const agentRouteResolved = !want || want.view !== 'agents' || routeAgents.ok
        const effectiveWant = routeResolved && agentRouteResolved ? want : null
        const candidates = appendSessionPage(page.items, exact.items).sort(
          (a, b) => b.updatedAt - a.updatedAt,
        )
        let sid = activeSessionIdRef.current
        let aid: string | null = null
        if (effectiveWant || !sid) {
          const picked = pickInitialSession({
            sessions: candidates,
            wantRoute: effectiveWant,
            draftedSessionIds: drafted,
            agentExists: (id) => routeAgents.items.some((agent) => agent.id === id),
          })
          sid = picked.sessionId
          aid = picked.agentId
          activeSessionIdRef.current = sid
          setActiveSessionId(sid)
          setActiveAgentId(aid)
        }
        if (effectiveWant && pendingRouteRef.current === want) pendingRouteRef.current = null
        const selected =
          candidates.find((session) => session.id === sid) ??
          sessionsRef.current.find((session) => session.id === sid)
        setSessions(mergeSelectedSession(page.items, selected))
        setSessionsTotal(page.total)
        setSessionsHasMore(page.hasMore)
        setSessionChipCounts(page.chipCounts ?? {})
        sessionsLimitRef.current = SESSIONS_PAGE_SIZE
        loadedPageSizeRef.current = page.items.length
      })
      .catch((e) => {
        if (listRequestGuardRef.current.isCurrent(token, listQueryIdentityRef.current)) {
          setError((e as Error).message)
        }
      })
      .finally(() => {
        if (listRequestGuardRef.current.isCurrent(token, listQueryIdentityRef.current)) {
          listReplacePendingRef.current = false
          setBootstrapping(false)
          runQueuedSessionRefresh()
        }
      })
  }, [activeWorkspaceId, chipsParam, runQueuedSessionRefresh, setError])

  // Agent CRUD events refresh only the roster. Re-running the workspace bootstrap
  // would unnecessarily clear the active session and transcript.
  useEffect(() => {
    if (agentsTick === lastAgentsTickRef.current) return
    lastAgentsTickRef.current = agentsTick
    if (!activeWorkspaceId) return
    api
      .listAgents()
      .then(setAllAgents)
      .catch((e) => setError((e as Error).message))
  }, [activeWorkspaceId, agentsTick, setError])

  // True when the stored default points at an agent that was DELETED — as
  // opposed to one that merely isn't in this workspace (the preference is
  // global, agents are per-workspace). Only allAgents can tell the two apart.
  const defaultAgentDeleted = useMemo(
    () => !!defaultAgentId && !!allAgents.find((a) => a.id === defaultAgentId)?.deleted,
    [allAgents, defaultAgentId],
  )

  const persistDefaultAgent = useCallback(
    async (id: string) => {
      if (defaultAgentSaveInFlightRef.current || defaultAgentIdRef.current === id) return
      const previousId = defaultAgentIdRef.current
      defaultAgentSaveInFlightRef.current = true
      setDefaultAgentSaveState('saving')
      const saved = await saveDefaultAgent({
        nextId: id,
        previousId,
        setCurrent: setCurrentDefaultAgent,
        persist: (nextId) => api.updateWorkspaceSettings({ defaultAgentId: nextId }),
        reportError: setError,
      })
      defaultAgentSaveInFlightRef.current = false
      setDefaultAgentSaveState(saved ? 'saved' : 'idle')
    },
    [setCurrentDefaultAgent, setError],
  )

  // Keep the default agent (for new sessions) valid. When the stored default is
  // absent or deleted from this workspace, self-heal to the first live agent and
  // persist the correction to the backend. A DELETED agent is left in place so the
  // empty state can say so: quietly starting the next chat with a different agent
  // than the user picked is worse than telling them their pick is gone.
  // System agents are never valid fallbacks — the backend rejects them as the
  // default (see workspace.ErrDefaultAgentSystem) — so skip them here too, or a
  // roster whose first live agent is a system agent retries this write forever.
  useEffect(() => {
    if (!wsSettingsLoaded || agents.length === 0 || defaultAgentDeleted) return
    if (!defaultAgentId || !agents.some((a) => a.id === defaultAgentId)) {
      const fallback = agents.find((a) => !a.system)?.id
      if (fallback) void persistDefaultAgent(fallback)
    }
  }, [agents, defaultAgentId, defaultAgentDeleted, persistDefaultAgent, wsSettingsLoaded])

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
    api
      .listArtifacts()
      .then((r) => setSessionArtifacts(r.items))
      .catch(() => {})
  }, [activeWorkspaceId, activeSessionId, meterRefresh])

  // Reload the session list (fresh order, updated times, unread flags) while
  // keeping the already-loaded window: refetch the same limit so a user who has
  // paged deeper does not get collapsed back to the first page on every event.
  const refreshSessions = useCallback(async (): Promise<boolean> => {
    // Do not invalidate the workspace/chip bootstrap request. It owns pending
    // deep-link resolution; replacing its request token here could leave the
    // route unresolved and the loading state stuck indefinitely.
    if (listReplacePendingRef.current) {
      listRefreshQueuedRef.current = true
      return false
    }
    const identity = listQueryIdentityRef.current
    const token = listRequestGuardRef.current.begin(identity)
    listReplacePendingRef.current = true
    try {
      const page = await api.listSessions({
        limit: sessionsLimitRef.current,
        chips: chipsParamRef.current,
        q: searchQueryRef.current,
      })
      if (!listRequestGuardRef.current.isCurrent(token, listQueryIdentityRef.current)) return false
      setSessions(withActiveSession(page.items))
      setSessionsTotal(page.total)
      setSessionsHasMore(page.hasMore)
      loadedPageSizeRef.current = page.items.length
      if (page.chipCounts) setSessionChipCounts(page.chipCounts)
      return true
    } catch {
      return false
    } finally {
      if (listRequestGuardRef.current.isCurrent(token, listQueryIdentityRef.current)) {
        listReplacePendingRef.current = false
        setBootstrapping(false)
        runQueuedSessionRefresh()
      }
    }
  }, [runQueuedSessionRefresh, withActiveSession])
  refreshSessionsRef.current = refreshSessions

  // Coalesced refresh for the live event stream. Every chat/session/board event
  // in the active workspace asks for the list again; during a multi-agent run
  // that is several requests a second, each one re-sorting the whole workspace
  // on the server and re-deriving every sidebar memo on a new array. One
  // trailing timer per burst is enough — the list is a projection, not the
  // transcript, so a few hundred milliseconds of lag is invisible.
  const refreshSoonTimerRef = useRef<number | null>(null)
  const refreshSessionsSoon = useCallback(() => {
    if (refreshSoonTimerRef.current !== null) return
    refreshSoonTimerRef.current = window.setTimeout(() => {
      refreshSoonTimerRef.current = null
      void refreshSessionsRef.current?.()
    }, SESSIONS_REFRESH_SETTLE_MS)
  }, [])
  useEffect(
    () => () => {
      if (refreshSoonTimerRef.current !== null) window.clearTimeout(refreshSoonTimerRef.current)
    },
    [],
  )

  // A changed search re-queries from the first page. The bootstrap effect owns
  // the initial load, so the very first (blank) value is skipped.
  const searchSeenRef = useRef(false)
  useEffect(() => {
    if (!searchSeenRef.current) {
      searchSeenRef.current = true
      if (!searchQuery.trim()) return
    }
    sessionsLimitRef.current = SESSIONS_PAGE_SIZE
    void refreshSessionsRef.current?.()
  }, [searchQuery])

  // Append the next page to the session list (sidebar "Daha fazla yükle").
  const loadMoreSessions = useCallback(() => {
    if (listReplacePendingRef.current) return
    // The offset is the loaded window's size in the SERVER's filtered ordering,
    // so an active session merged in behind the filter must not shift it.
    const offset = loadedPageSizeRef.current
    if (offset === 0) return
    const identity = listQueryIdentityRef.current
    const token = listRequestGuardRef.current.begin(identity)
    api
      .listSessions({
        limit: SESSIONS_PAGE_SIZE,
        offset,
        chips: chipsParamRef.current,
        q: searchQueryRef.current,
      })
      .then((page) => {
        if (!listRequestGuardRef.current.isCurrent(token, listQueryIdentityRef.current)) return
        if (page.chipCounts) setSessionChipCounts(page.chipCounts)
        if (page.items.length === 0) {
          setSessionsHasMore(false)
          return
        }
        setSessions((prev) => appendSessionPage(prev, page.items))
        loadedPageSizeRef.current = offset + page.items.length
        sessionsLimitRef.current = loadedPageSizeRef.current
        setSessionsTotal(page.total)
        setSessionsHasMore(page.hasMore)
      })
      .catch(() => {})
  }, [])

  // Change which agent answers the active chat session (the composer's mandatory
  // agent dropdown — "@mention" routing was removed). Updates the local selection
  // immediately and persists it to the session so it survives a reload.
  const changeChatAgent = useCallback(
    (id: string) => {
      setActiveAgentId(id)
      const sid = activeSessionIdRef.current
      if (!sid) return
      api
        .setSessionAgent(sid, id)
        .then(() =>
          setSessions((prev) => prev.map((s) => (s.id === sid ? { ...s, agentId: id } : s))),
        )
        .catch((e) => setError((e as Error).message))
    },
    [setError],
  )

  // Select a session: reflect its default agent and clear its unread flag.
  // When a cross-session search result is clicked, the target message id is
  // stashed here so MessageList scrolls to (and briefly highlights) it once the
  // session's transcript has loaded. Cleared after the scroll is consumed.
  const [scrollToMsgId, setScrollToMsgId] = useState<string | null>(null)

  // A freshly-created "new chat" that has received no message yet. If the user
  // leaves it (opens another session or a new chat) without ever sending anything,
  // it is auto-deleted on the way out so empty abandoned chats don't pile up.
  const freshEmptyRef = useRef<string | null>(null)
  // Exact-ID selections can overlap (hash navigation and search results). Only
  // the newest lookup may change the open transcript.
  const sessionSelectSeqRef = useRef(0)

  // discardEmptyFresh deletes the tracked fresh session when it is the one being
  // left AND neither its transcript nor its active-workspace draft has content.
  // leavingId is the session being navigated away from.
  const discardEmptyFresh = useCallback(
    (leavingId: string | null) => {
      const id = freshEmptyRef.current
      if (!id || id !== leavingId) return
      freshEmptyRef.current = null
      if (
        !shouldDiscardFreshSession({
          freshSessionId: id,
          leavingSessionId: leavingId,
          messageCount: messagesRef.current.length,
          draft: readSessionDraftState(id),
        })
      )
        return
      api.deleteSession(id).catch(() => {})
      setSessions((prev) => prev.filter((s) => s.id !== id))
    },
    [messagesRef],
  )

  const commitSessionSelection = useCallback(
    (sess: Session, messageId?: string) => {
      const id = sess.id
      // Leaving the current session: clean it up if it was an unused new chat.
      if (id !== activeSessionIdRef.current) discardEmptyFresh(activeSessionIdRef.current)
      activeSessionIdRef.current = id
      setActiveSessionId(id)
      setScrollToMsgId(messageId ?? null)
      setActiveAgentId(sess.agentId)
      // Optimistically clear unread, then persist on the backend.
      setSessions((prev) => prev.map((s) => (s.id === id ? { ...s, unread: false } : s)))
      api.markSessionRead(id).catch(() => {})
    },
    [discardEmptyFresh],
  )

  const selectSession = useCallback(
    (id: string, messageId?: string) => {
      const seq = ++sessionSelectSeqRef.current
      const known = sessionsRef.current.find((session) => session.id === id)
      if (known) {
        commitSessionSelection(known, messageId)
        return
      }

      // Hash/back-forward navigation can target a valid session outside the
      // current filtered page. Resolve it exactly before opening the transcript.
      const workspaceID = activeWorkspaceIdRef.current
      if (!workspaceID) return
      void api
        .getSessionsByIds([id])
        .then((items) => {
          if (sessionSelectSeqRef.current !== seq || activeWorkspaceIdRef.current !== workspaceID)
            return
          const session = items.find((item) => item.id === id)
          if (!session) {
            setError('Session not found.')
            return
          }
          setSessions((prev) => mergeSelectedSession(prev, session))
          commitSessionSelection(session, messageId)
        })
        .catch((e) => {
          if (sessionSelectSeqRef.current === seq && activeWorkspaceIdRef.current === workspaceID) {
            setError((e as Error).message)
          }
        })
    },
    [commitSessionSelection, setError],
  )

  // ---- per-session actions (settings menu) ----
  const renameSession = useCallback(
    async (id: string, title: string) => {
      try {
        await api.setSessionTitle(id, title)
        setSessions((prev) => prev.map((s) => (s.id === id ? { ...s, title } : s)))
      } catch (e) {
        setError((e as Error).message)
      }
    },
    [setError],
  )

  // Archive / restore a session (the sidebar Active/Archived filter). Archiving
  // changes membership and ordering in the server-filtered set, so refetch the
  // whole currently loaded window instead of shifting its offset locally.
  const setSessionArchived = useCallback(
    async (id: string, archived: boolean) => {
      try {
        await api.setSessionState(id, archived ? 'archived' : 'active')
        if (archived && activeSessionIdRef.current === id) {
          const fallback = sessionsRef.current.find(
            (session) => session.id !== id && session.state !== 'archived',
          )
          const fallbackID = fallback?.id ?? null
          activeSessionIdRef.current = fallbackID
          setActiveSessionId(fallbackID)
          setActiveAgentId(fallback?.agentId ?? null)
        }
        await refreshSessions()
      } catch (e) {
        setError((e as Error).message)
      }
    },
    [refreshSessions, setError],
  )

  // Pin / unpin a session (sidebar). Optimistic; ListSessions floats pinned to top.
  const setSessionPinned = useCallback(
    async (id: string, pinned: boolean) => {
      setSessions((prev) => prev.map((s) => (s.id === id ? { ...s, pinned } : s)))
      try {
        await api.setSessionPinned(id, pinned)
        refreshSessions()
      } catch (e) {
        setError((e as Error).message)
      }
    },
    [refreshSessions, setError],
  )

  const copySessionPath = useCallback(
    async (id: string) => {
      try {
        const { path } = await api.sessionPath(id)
        await copyToClipboard(path, 'Yolu kopyalayın (Ctrl+C, Enter):')
      } catch (e) {
        setError((e as Error).message)
      }
    },
    [setError],
  )

  const deleteSession = useCallback(
    async (id: string) => {
      if (id === freshEmptyRef.current) freshEmptyRef.current = null
      try {
        await deleteSessionAndRefresh(id, api.deleteSession, refreshSessions)
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
    [activeSessionId, refreshSessions, setError],
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
  const rateMessage = useCallback(
    async (id: string, rating: number) => {
      const sid = activeSessionIdRef.current
      if (!sid) return
      setMessages((prev) =>
        prev.map((m) =>
          m.id === id
            ? {
                ...m,
                feedback: rating === 0 ? undefined : { rating, at: Math.floor(Date.now() / 1000) },
              }
            : m,
        ),
      )
      try {
        await api.setMessageFeedback(sid, id, rating)
      } catch (e) {
        setError((e as Error).message)
      }
    },
    [setError],
  )

  // Pick the default agent for NEW sessions (from the roster). Persists to
  // the backend per-workspace so it survives reloads and never leaks across
  // workspaces. The local state updates immediately for instant UI feedback.
  const pickDefaultAgent = persistDefaultAgent

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
  const openAgentSettings = useCallback(
    (id: string) => {
      setActiveAgentId(id)
      setView('agents')
    },
    [setView],
  )

  const createAgent = useCallback(
    async (
      name: string,
      soul: string,
      provider: string,
      model?: string,
      coordinator?: { mode: boolean; workflow: string; prompt: string },
      // When set, the agent is created as a child of this agent: provider /
      // model / thinking / coordinator settings are inherited, so the values
      // above are ignored by the server for a derived create.
      parentId?: string,
    ) => {
      try {
        const agent = await api.createAgent({
          name,
          soul,
          provider,
          model,
          // The backend has no default for this. 'off' is what a new agent used
          // to get implicitly: no extended reasoning natively, and on the CLI
          // path it maps to the same effortLevel the blank value did. The user
          // raises it from the agent settings form.
          thinkingLevel: 'off',
          coordinatorMode: coordinator?.mode,
          coordinatorWorkflow: coordinator?.workflow,
          coordinatorPrompt: coordinator?.prompt,
          parentId: parentId || undefined,
        })
        setAllAgents((prev) => [agent, ...prev])
        // Focus the new agent (so Agents/Tools views select it) but do NOT make
        // it the default for new chats: creating an agent must not silently
        // change the user's chosen default. The very first agent still becomes
        // the default via the "keep default valid" effect above, which fills in
        // agents[0] when no valid default is set.
        setActiveAgentId(agent.id)
      } catch (e) {
        setError((e as Error).message)
      }
    },
    [setError],
  )

  const updateAgent = useCallback(
    async (id: string, patch: AgentPatch): Promise<{ agent: Agent; warning?: string }> => {
      const { agent, warning } = await api.updateAgent(id, patch)
      setAllAgents((prev) => prev.map((a) => (a.id === id ? agent : a)))
      return { agent, warning }
    },
    [],
  )

  // Duplicate an agent: the server clones the whole profile + tool config into a
  // new "(kopya)". Prepend it to the roster and focus it — like createAgent, this
  // does NOT touch the default agent for new chats.
  const duplicateAgent = useCallback(
    async (id: string) => {
      try {
        const clone = await api.duplicateAgent(id)
        setAllAgents((prev) => [clone, ...prev])
        setActiveAgentId(clone.id)
        return clone.id
      } catch (e) {
        setError((e as Error).message)
      }
    },
    [setError],
  )

  // Derive a child that inherits every field from `id` (bindRole: take over
  // the parent's system role — how a locked built-in is customised). Like
  // duplicate, the new agent is focused but never made the chat default.
  const deriveAgent = useCallback(
    async (id: string, opts: { name?: string; bindRole?: boolean } = {}) => {
      try {
        const child = await api.deriveAgent(id, opts)
        setAllAgents((prev) => [child, ...prev])
        setActiveAgentId(child.id)
        return child.id
      } catch (e) {
        setError((e as Error).message)
      }
    },
    [setError],
  )

  // Delete an agent. The server soft-deletes it: schedules and owned tasks go,
  // the agent row and its SESSIONS stay. So mark it deleted here rather than
  // dropping it — `agents` (the live subset) loses it immediately, while history
  // can still resolve its name and avatar. The server refuses (409) while the
  // agent has a turn in flight; that message reaches the user through setError.
  const deleteAgent = useCallback(
    async (id: string) => {
      try {
        await api.deleteAgent(id)
        setAllAgents((prev) =>
          prev.map((a) =>
            a.id === id ? { ...a, deleted: true, deletedAt: Date.now() / 1000 } : a,
          ),
        )
        // Its sessions survive, but a cascade dropped its schedules/tasks — refresh
        // the session list so any state derived from those is current.
        await refreshSessions()
      } catch (e) {
        setError((e as Error).message)
      }
    },
    [refreshSessions, setError],
  )

  const newSession = useCallback(async () => {
    const workspaceID = activeWorkspaceIdRef.current
    if (!workspaceID) return
    const aid = defaultAgentId ?? agents[0]?.id
    if (!aid) {
      setError('Create an agent before starting a new session.')
      return
    }
    // Discard the previous new chat if it was left empty, before opening another.
    discardEmptyFresh(activeSessionIdRef.current)
    try {
      const s = await api.createSession(aid)
      // The request belongs to its originating workspace. A late response must
      // not inject that session into a workspace the user switched to meanwhile.
      if (activeWorkspaceIdRef.current !== workspaceID) return
      sessionSelectSeqRef.current += 1
      setSessions((prev) => [s, ...prev])
      activeSessionIdRef.current = s.id
      setActiveSessionId(s.id)
      setActiveAgentId(s.agentId)
      setMessages([])
      // Track it as a fresh, unused chat (cleared once a message is sent / it's left).
      freshEmptyRef.current = s.id
      // Mark this new chat as the one that should auto-focus the input (the composer
      // focuses only when the active session matches this id).
      setFocusSessionId(s.id)
    } catch (e) {
      if (activeWorkspaceIdRef.current === workspaceID) setError((e as Error).message)
    }
  }, [defaultAgentId, agents, discardEmptyFresh, activeSessionIdRef, setError])

  // Regenerate a session's title from its conversation on demand.
  const regenerateSessionTitle = useCallback(
    async (sessionId: string) => {
      try {
        const { title } = await api.generateSessionTitle(sessionId)
        setSessions((prev) => prev.map((s) => (s.id === sessionId ? { ...s, title } : s)))
      } catch (e) {
        setError((e as Error).message)
      }
    },
    [setError],
  )

  // Whether the open session accepts new user turns. Task / flow / schedule
  // transcripts are read-only run logs: the composer is hidden for them. An
  // unknown id (list not yet loaded) is treated as writable so the composer does
  // not flicker away mid-load; the `bootstrapping` guard covers that window.
  const activeSessionWritable = useMemo(() => {
    if (!activeSessionId) return true
    const s = sessions.find((x) => x.id === activeSessionId)
    return s ? isWritableSessionKind(s.kind) : true
  }, [sessions, activeSessionId])

  return {
    // state
    // agents = live only (pickers, rosters, defaults).
    // allAgents = live + deleted, for resolving the author of past history.
    agents,
    allAgents,
    setAgents: setAllAgents,
    sessions,
    setSessions,
    sessionsTotal,
    sessionsHasMore,
    sessionChipCounts,
    loadMoreSessions,
    messages,
    setMessages,
    activeAgentId,
    activeSessionId,
    bootstrapping,
    messagesLoading,
    activeSessionWritable,
    sessionArtifacts,
    defaultAgentId,
    defaultAgentSaveState,
    defaultAgentDeleted,
    composerKey,
    setComposerKey,
    focusSessionId,
    setFocusSessionId,
    scrollToMsgId,
    setScrollToMsgId,
    meterRefresh,
    setMeterRefresh,
    bumpMeter,
    // refs
    activeSessionIdRef,
    messagesRef,
    chatRef,
    pendingRouteRef,
    // actions
    refreshSessions,
    refreshSessionsSoon,
    changeChatAgent,
    selectSession,
    renameSession,
    setSessionArchived,
    setSessionPinned,
    copySessionPath,
    deleteSession,
    deleteMessage,
    rateMessage,
    pickDefaultAgent,
    pickAgent,
    focusAgent,
    openAgentSettings,
    createAgent,
    updateAgent,
    duplicateAgent,
    deriveAgent,
    deleteAgent,
    newSession,
    regenerateSessionTitle,
  }
}
