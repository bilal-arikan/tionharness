// App is the composition root: it wires the app-level hooks (workspaces,
// appearance, sessions controller, chat stream, SSE events, URL routing) into
// the shell layout (nav rail, per-view panels, modals). The per-concern logic
// lives in the app/use*.ts hooks; the view metadata in viewRegistry.tsx.
import { useCallback, useEffect, useMemo, useState, Suspense } from 'react'
import { api } from '@/api'
import { LoadingState, Toaster, toast, CommandPalette, type Command } from '@/shared/components'
import { NavRail, type View } from './NavRail'
import { MobileNavBar } from './MobileNavBar'
import { SplashScreen } from './SplashScreen'
import { AppHeader } from './AppHeader'
import { FlowsPanel, NetworkPanel, ExplorerView } from './lazyPanels'
import { HEADERLESS_VIEWS, SPLASH_MIN_MS, VIEW_TITLE } from './viewRegistry'
import { INITIAL_ROUTE, useAppNavigation } from './useAppNavigation'
import { useAppearance } from './useAppearance'
import { useAppEvents } from './useAppEvents'
import { useDeepLinks } from './useDeepLinks'
import { useSessionsController } from './useSessionsController'
import { useExecutionRuntime } from './useExecutionRuntime'
import { useWorkspaces } from './useWorkspaces'
import { useActivity } from './useActivity'
import { useWorkspaceActivity } from './useWorkspaceActivity'
import { useUnreadViews } from './useUnreadViews'
import { useUnreadBadge } from './useUnreadBadge'
import { setSessionState } from '@/shared/hooks/useSessionState'
import { ChatView } from '@/features/chat/ChatView'
import { useChatStream } from '@/features/chat/useChatStream'
import { writeSessionDraft } from '@/features/chat/useSessionDraft'
import { SessionsSidebar } from '@/features/sessions/SessionsSidebar'
import { SessionsOverview } from '@/features/sessions/SessionsOverview'
import { SessionDetailPanel } from '@/features/sessions/SessionDetailPanel'
import { CoordinatorPanel } from '@/features/sessions/CoordinatorPanel'
import { SessionContextModal } from '@/features/sessions/SessionContextModal'
import { SessionDebugModal } from '@/features/sessions/SessionDebugModal'
import { SessionFlowInline } from '@/features/flows/SessionFlowInline'
import { AgentsView } from '@/features/agents/AgentsView'
import { TaskBoard } from '@/features/tasks/TaskBoard'
import { AutomationBoard } from '@/features/schedules/AutomationBoard'
import { ArtifactsPanel } from '@/features/artifacts/ArtifactsPanel'
import { ArtifactPreviewModal } from '@/features/artifacts/ArtifactPreviewModal'
import { SkillsPanel } from '@/features/skills/SkillsPanel'
import { ToolsPanel as ToolCatalogPanel } from '@/features/tools/ToolsPanel'
import { MarketPanel } from '@/features/market/MarketPanel'
import { BudgetPanel } from '@/features/budget/BudgetPanel'
import { DashboardPanel } from '@/features/dashboard/DashboardPanel'
import { LogsPanel } from '@/features/logs/LogsPanel'
import { InsightPanel } from '@/features/insight/InsightPanel'
import { SettingsPanel } from '@/features/settings/SettingsPanel'
import { WorkspaceView } from '@/features/workspace/WorkspaceView'
import { OnboardingScreen } from '@/features/workspace/OnboardingScreen'
import { ClaudeAuthGate } from '@/features/workspace/ClaudeAuthGate'
import { WorkspaceRecommendations } from '@/features/workspace/WorkspaceRecommendations'
import { useDirtyViews } from '@/shared/lib/dirtySignals'
import { useIsMobile } from '@/shared/hooks/useMediaQuery'
import { useCollapsibleList } from '@/shared/hooks/useCollapsibleList'
import { isImagePath, mediaUrl } from '@/shared/lib/paths'
import { copyToClipboard } from '@/shared/lib/clipboard'
import { initServerTts, initTtsUnlock } from '@/shared/lib/tts'
import { initServerStt } from '@/shared/lib/stt'

export default function App() {
  // Error reporting funnels every `onError(msg)` sink into a toast. Kept under the
  // old `setError` name/signature so the ~20 `onError={setError}` call sites and
  // the setError-taking hooks (useWorkspaces/useAppearance/…) are unchanged.
  const setError = useCallback((msg: string | null) => {
    if (msg) toast.error(msg)
  }, [])
  const [view, setView] = useState<View>(INITIAL_ROUTE.view)
  // ⌘K / Ctrl+K command palette visibility.
  const [paletteOpen, setPaletteOpen] = useState(false)
  // Bumped whenever an agent changes app settings (the `settings` SSE event), so
  // an open Settings screen reloads to reflect the change.
  const [settingsNonce, setSettingsNonce] = useState(0)

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
    attachWorkspace,
    deleteActiveWorkspace,
    refreshWorkspaces,
  } = useWorkspaces(setError)

  // Global ⌘K / Ctrl+K toggles the command palette. Registered once at the shell.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && (e.key === 'k' || e.key === 'K')) {
        e.preventDefault()
        setPaletteOpen((o) => !o)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  // Palette command list: jump to any view + switch to any workspace. Kept lean
  // and fully driven by existing handlers (setView / switchWorkspace) so it can
  // never drift out of sync with what those actions actually do.
  const paletteCommands = useMemo<Command[]>(() => {
    const nav = (Object.keys(VIEW_TITLE) as View[]).map((v) => ({
      id: `view:${v}`,
      label: VIEW_TITLE[v],
      group: 'Git',
      keywords: v,
      run: () => setView(v),
    }))
    const ws = workspaces.map((w) => ({
      id: `ws:${w.id}`,
      label: w.name || 'İsimsiz',
      group: 'Workspace',
      keywords: 'workspace çalışma alanı',
      run: () => switchWorkspace(w.id),
    }))
    return [...nav, ...ws]
  }, [workspaces, switchWorkspace])

  // Post-create claude-cli readiness gate: bumped once per successful workspace
  // creation so ClaudeAuthGate (re-)probes the new workspace's login state and
  // either offers the auth popup or steers to the Providers screen.
  // requireNoProvider distinguishes the two gate reasons:
  //   - create/attach (false): always probe/offer login for the new workspace.
  //   - chat-open (true): only nag when the workspace has NO usable provider at
  //     all, so users who configured a provider are never bothered.
  const [claudeGate, setClaudeGate] = useState<{ nonce: number; requireNoProvider: boolean }>({
    nonce: 0,
    requireNoProvider: false,
  })
  const bumpClaudeGate = useCallback((requireNoProvider: boolean) => {
    setClaudeGate((g) => ({ nonce: g.nonce + 1, requireNoProvider }))
  }, [])
  // Separate trigger for the post-create advisory cards (WorkspaceRecommendations).
  // Bumped ONLY on a successful create/attach — never on chat-open — so the user is
  // offered tool wiring once per new workspace, not nagged every time they open chat.
  const [recsTrigger, setRecsTrigger] = useState(0)
  const handleCreateWorkspace = useCallback(
    async (data: Parameters<typeof createWorkspace>[0]) => {
      const created = await createWorkspace(data)
      if (created) {
        bumpClaudeGate(false)
        setRecsTrigger((n) => n + 1)
      }
    },
    [createWorkspace, bumpClaudeGate],
  )
  // Adopting an existing workspace folder runs the same claude-cli gate: the
  // attached workspace may have its own (unauthenticated) claude-home. Preserves
  // attachWorkspace's throw-on-failure contract so the onboarding screen still
  // renders inline validation errors.
  const handleAttachWorkspace = useCallback(
    async (path: string) => {
      const attached = await attachWorkspace(path)
      if (attached) {
        bumpClaudeGate(false)
        setRecsTrigger((n) => n + 1)
      }
      return attached
    },
    [attachWorkspace, bumpClaudeGate],
  )
  // Re-nag on every chat-screen entry (and workspace switch while in chat): when
  // the workspace has no provider configured and claude-cli is present but not
  // logged in, the gate re-shows the auth popup. The "no provider" precondition
  // lives in the gate, checked cheaply before the (expensive) login probe.
  useEffect(() => {
    if (view === 'chat') bumpClaudeGate(true)
  }, [view, activeWorkspaceId, bumpClaudeGate])

  // First-run gating. `hadSetupAtBoot` is captured ONCE at mount: a returning
  // user (who has had at least one workspace before) skips the splash entirely
  // and lands in the app shell, while a truly fresh install shows the splash
  // during the initial load and then the onboarding screen. The flag is set the
  // moment a workspace exists, so the splash never reappears after setup.
  const [hadSetupAtBoot] = useState(() => {
    try {
      return localStorage.getItem('tionswarm.hasSetup') === '1'
    } catch {
      return false
    }
  })
  useEffect(() => {
    if (workspaces.length > 0) {
      try {
        localStorage.setItem('tionswarm.hasSetup', '1')
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

  // Portrait-phone layout flag: below Tailwind's `md` breakpoint the desktop rail
  // and persistent sidebars collapse into a bottom nav + slide-in drawers.
  const isMobile = useIsMobile()
  // Mobile-only: the left list column (chat sessions OR the agents roster)
  // is a slide-in drawer instead of an always-visible column. Opened via the
  // header hamburger; closed on select. One flag shared across list-bearing views.
  const [mobileListOpen, setMobileListOpen] = useState(false)

  // App-headed list screens (workspace / settings) own their category-rail
  // collapse here so the app header's toggle button and the panel share one flag.
  const workspaceNav = useCollapsibleList('tionswarm.workspaceNavOpen')
  const settingsNav = useCollapsibleList('tionswarm.settingsNavOpen')

  // Next-turn context preview modal (opens straight from the chat header).
  const [ctxPreviewOpen, setCtxPreviewOpen] = useState(false)
  // Debug/observability modal (opened from the chat header's "Debug" button).
  const [debugOpen, setDebugOpen] = useState(false)
  // Session-as-flow inline view (chat header's "Akış" toggle): the current
  // transcript reified into a completed, read-only flow run, shown in place of the
  // chat transcript (session→flow bridge). Reset when the session changes.
  const [sessionFlowOpen, setSessionFlowOpen] = useState(false)
  // Right-hand session detail panel visibility (persisted).
  const [detailOpen, setDetailOpen] = useState(
    () => localStorage.getItem('tionswarm.detailOpen') === '1',
  )
  const toggleDetail = useCallback(() => {
    setDetailOpen((v) => {
      const next = !v
      localStorage.setItem('tionswarm.detailOpen', next ? '1' : '0')
      return next
    })
  }, [])
  // Coordination drawer (chat header's "Coord" button): a right-anchored side sheet
  // hosting the coordination UI (worker roster, workflow, tree). Not persisted — it
  // is an on-demand overlay, not a docked column like the detail panel.
  const [coordOpen, setCoordOpen] = useState(false)

  // Theme / keep-awake / desktop-notification preferences.
  const { applyClientPrefs, onAppearanceSaved, onWorkspaceNotifySaved, notifyEnabled } =
    useAppearance(activeWorkspaceId, setError)

  // Per-view deep-link targets + cross-view "open X" helpers.
  const links = useDeepLinks(setView)

  // Workspace-scoped data model: agents, sessions, transcript + their actions.
  const ctl = useSessionsController({
    activeWorkspaceId,
    setError,
    setView,
  })
  // Leave the inline session-as-flow view when the active session changes.
  useEffect(() => setSessionFlowOpen(false), [ctl.activeSessionId])

  // Live/last-run facts per session (GET /api/executions): the sidebar's pulse
  // dot + status pill and the bulk overview table's rows. Same DB.ListSessions
  // source as the session list, merely enriched with running/status.
  const { executions, runtimeById } = useExecutionRuntime(activeWorkspaceId)
  // Bulk sessions overview overlay (the searchable/sortable table of every
  // session), opened from the sidebar's "Oturumlar" button.
  const [overviewOpen, setOverviewOpen] = useState(false)

  // Clicking a file path: open images inline (new tab via the file server),
  // copy other paths to the clipboard as a best-effort action.
  const openFile = useCallback((path: string) => {
    if (isImagePath(path)) {
      window.open(mediaUrl(path), '_blank')
    } else {
      void copyToClipboard(path)
    }
  }, [])

  // Chat-turn streaming machinery (send loop, interventions, slash commands).
  const chat = useChatStream({
    agents: ctl.agents,
    sessions: ctl.sessions,
    activeSessionId: ctl.activeSessionId,
    activeAgentId: ctl.activeAgentId,
    activeSessionIdRef: ctl.activeSessionIdRef,
    messagesRef: ctl.messagesRef,
    notifyEnabled,
    setMessages: ctl.setMessages,
    setError,
    selectSession: ctl.selectSession,
    refreshSessions: ctl.refreshSessions,
    bumpMeter: ctl.bumpMeter,
  })
  // Publish the live chat handle for the controller's messages-load effect.
  // Effect-time assignment is safe: the effect reads the ref inside an async
  // listMessages callback, which always resolves after effects have flushed.
  const { chatRef } = ctl
  useEffect(() => {
    chatRef.current = chat
  })

  // Open session, resolved once for the ChatView props below. A flow run log
  // (kind 'flow') gets a shortcut to its flow's run history (B#4): a flow
  // session's sourceId IS its flow id, and openFlowRun opens the Flows screen on
  // that flow's runs. Undefined for every other kind, so the link surfaces only
  // where it resolves.
  const activeSession = ctl.sessions.find((s) => s.id === ctl.activeSessionId)
  const openRunHistory =
    activeSession?.kind === 'flow' && activeSession.sourceId
      ? () => links.openFlowRun(activeSession.sourceId as string)
      : undefined

  // handleRewind rewinds the conversation to a message (via the "/rewind" dialog
  // or a user bubble's ⟲ hover action): truncate to that checkpoint, then drop the
  // removed prompt back into the composer (draft write + remount) for a re-try.
  const handleRewind = useCallback(
    async (id: string) => {
      const text = await chat.rewindTo(id)
      if (text) {
        writeSessionDraft(ctl.activeSessionId ?? undefined, text)
        ctl.setComposerKey((k) => k + 1)
        // Rewind restores a prompt for a deliberate re-try → land the cursor in the
        // (remounted) input, matching the pre-existing rewind behaviour.
        ctl.setFocusSessionId(ctl.activeSessionId)
      }
      return text
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [chat, ctl.activeSessionId],
  )

  // After a reload (or workspace switch), re-seat the "thinking" indicators on the
  // server's authoritative list of detached turns still in flight for THIS
  // workspace — restoring them for a mid-turn refresh AND dropping any latch that
  // outlived its turn (see reconcileActive: the completion feed only clears while
  // the session's own workspace is active, and session ids repeat across stores).
  // Whichever workspace is entered last wins: a slow response from a workspace we
  // already left must not overwrite the current one's indicators.
  const reconcileActive = chat.reconcileActive
  useEffect(() => {
    if (!activeWorkspaceId) return
    let cancelled = false
    api
      .activeSessions()
      .then((ids) => {
        if (!cancelled) reconcileActive(ids)
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [activeWorkspaceId, reconcileActive])

  // Opening a session must show at once that its turn is still running, instead of
  // ending on our own message as if the agent had gone quiet. The per-session hub
  // subscription eventually replays the in-flight turn, but only once a frame of it
  // has been emitted — a turn still queued behind another, or one whose first
  // assistant frame hasn't landed, produces nothing to replay. So seed the
  // indicator from the server's authoritative list. ADD-only (markPending, not
  // reconcileActive): a turn this window just enqueued may not be registered
  // server-side yet, and pruning here would blank its indicator.
  const markPending = chat.markPending
  const openedSessionId = ctl.activeSessionId
  useEffect(() => {
    if (!openedSessionId) return
    let cancelled = false
    api
      .activeSessions()
      .then((ids) => {
        if (!cancelled) markPending(ids.filter((id) => id === openedSessionId))
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [openedSessionId, markPending])

  // Per-view "work in progress" flags for the nav-rail busy indicators.
  // The instant local chat signal must be workspace-scoped: useChatStream lives
  // at App level (it does NOT remount on a workspace switch), so its
  // streamingSessions set keeps a turn started in workspace A even after we
  // switch into B. Feeding raw `size > 0` would light B's chat dot for A's work
  // — the very cross-workspace leak this indicator is meant to avoid. Gate it on
  // sessions that belong to the ACTIVE workspace (ctl.sessions is scoped);
  // any other same-workspace stream is still covered by useActivity's poll.
  const chatBusyLocal = useMemo(
    () => ctl.sessions.some((s) => chat.streamingSessions.has(s.id)),
    [ctl.sessions, chat.streamingSessions],
  )
  const busyViews = useActivity(activeWorkspaceId, chatBusyLocal)
  // Cross-workspace live-run flags (GET /api/workspaces/activity): the set of
  // workspaces — including non-active ones — that currently have a run in flight,
  // so the switcher pulses a "çalışıyor" dot on each. Complements busyViews, which
  // only covers the active workspace's per-view breakdown.
  const busyWorkspaceIds = useWorkspaceActivity(activeWorkspaceId)
  // Per-view unread (unseen activity from the SSE feed) + unsaved-edit (dirty)
  // signals — the other two channels of the generic nav notification system.
  const { unreadViews, markViewUnread, markViewRead } = useUnreadViews(activeWorkspaceId)
  const dirtyViews = useDirtyViews()

  // App-wide SSE feed: notifications, badges, live refreshes, turn-step frames.
  useAppEvents({
    chat,
    view,
    activeSessionId: ctl.activeSessionId,
    notifyEnabled,
    applyClientPrefs,
    refreshWorkspaces,
    refreshSessions: ctl.refreshSessions,
    setMessages: ctl.setMessages,
    setMeterRefresh: ctl.setMeterRefresh,
    setSettingsNonce,
    markWorkspaceUnread,
    markWorkspaceRead,
    markViewUnread,
  })

  // Boot the read-aloud engine: probe for the optional server-side Piper TTS
  // (so 'auto' prefers it) and arm the mobile autoplay unlock on first gesture.
  useEffect(() => {
    void initServerTts()
    void initServerStt()
    initTtsUnlock()
  }, [])

  // Navigation guard: if the current screen has unsaved edits, confirm before
  // switching to another view so those edits are not silently lost. Returns true
  // when it is safe to proceed. Same-view selects always pass.
  const confirmLeaveIfDirty = useCallback(
    (next: View): boolean => {
      if (next === view) return true
      if (!dirtyViews.has(view)) return true
      return window.confirm(
        'Bu sayfada kaydedilmemiş değişiklikler var. Kaydetmeden ayrılmak istiyor musunuz?',
      )
    },
    [view, dirtyViews],
  )
  const selectView = useCallback(
    (next: View) => {
      if (confirmLeaveIfDirty(next)) {
        setView(next)
        // Leaving a list-bearing view closes the mobile list drawer so it never
        // lingers over another screen.
        setMobileListOpen(false)
      }
    },
    [confirmLeaveIfDirty],
  )

  // Open the Skills screen focused on one skill. SkillsPanel is mounted by the
  // view switch and reads its selection from the session-state store in its
  // initialiser, so the seed has to be written before we navigate.
  const openSkill = useCallback(
    (slug: string) => {
      setSessionState('skills.activeSlug', slug)
      selectView('skills')
    },
    [selectView],
  )

  // Warn on tab close / reload (browser-native prompt) whenever any screen has
  // unsaved edits. The message text is controlled by the browser.
  useEffect(() => {
    if (dirtyViews.size === 0) return
    const handler = (e: BeforeUnloadEvent) => {
      e.preventDefault()
      e.returnValue = ''
    }
    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [dirtyViews])

  // Clear a view's unread badge as soon as it is shown (in this window).
  useEffect(() => {
    markViewRead(view)
  }, [view, markViewRead])

  // Total unseen items, surfaced on the tab title (while unfocused) + OS taskbar
  // badge. Sum of: unread chat sessions (active ws) + other workspaces with
  // activity + non-chat view badges (chat is already counted via sessions).
  const unreadTotal = useMemo(() => {
    const sess = ctl.sessions.filter((s) => s.unread).length
    const views = [...unreadViews].filter((v) => v !== 'chat').length
    return sess + unreadWs.size + views
  }, [ctl.sessions, unreadViews, unreadWs])
  useUnreadBadge(unreadTotal)

  // URL ↔ state sync (canonical route + applyRoute for back/forward/deep links).
  useAppNavigation({
    view,
    setView,
    activeWorkspaceId,
    activeSessionId: ctl.activeSessionId,
    activeAgentId: ctl.activeAgentId,
    artifactTarget: links.artifactTarget,
    scheduleTarget: links.scheduleTarget,
    settingsCat: links.settingsCat,
    workspaceTab: links.workspaceTab,
    insightTab: links.insightTab,
    explorerNode: links.explorerNode,
    flowsTab: links.flowsTab,
    sessionListTab: links.sessionListTab,
    sessionKindTab: links.sessionKindTab,
    pendingRouteRef: ctl.pendingRouteRef,
    switchWorkspace,
    selectSession: ctl.selectSession,
    focusAgent: ctl.focusAgent,
    setArtifactTarget: links.setArtifactTarget,
    setScheduleTarget: links.setScheduleTarget,
    setSettingsCat: links.setSettingsCat,
    setWorkspaceTab: links.setWorkspaceTab,
    setInsightTab: links.setInsightTab,
    setExplorerNode: links.setExplorerNode,
    setFlowsTab: links.setFlowsTab,
    setSessionListTab: links.setSessionListTab,
    setSessionKindTab: links.setSessionKindTab,
  })

  // ---- First-run gating (must stay AFTER every hook above) ----
  // Fresh install → splash while the list is loading AND until the minimum
  // duration elapses (only when no prior setup). Returning users skip it.
  if (!hadSetupAtBoot && (wsLoading || !minSplashElapsed)) {
    return <SplashScreen />
  }
  // List resolved and there are zero workspaces → onboarding. Closing the popup
  // without creating one provisions nothing (no default workspace).
  if (!wsLoading && workspaces.length === 0) {
    return <OnboardingScreen onCreate={handleCreateWorkspace} onAttach={handleAttachWorkspace} />
  }

  return (
    <div className="flex h-full">
      <NavRail
        view={view}
        onSelectView={selectView}
        workspaces={workspaces}
        activeWorkspaceId={activeWorkspaceId}
        unreadWorkspaceIds={unreadWs}
        busyWorkspaceIds={busyWorkspaceIds}
        busyViews={busyViews}
        unreadViews={unreadViews}
        dirtyViews={dirtyViews as Set<View>}
        favoriteWorkspaceId={favoriteWorkspaceId}
        onSetFavoriteWorkspace={setFavoriteWorkspace}
        onSwitchWorkspace={switchWorkspace}
        onCreateWorkspace={handleCreateWorkspace}
      />

      {/* The agent/session list only applies to agent-scoped views. Board and
          schedules are workspace-scoped, so the list is hidden there. */}
      {/* Chat: a sessions-only list (agents now live in their own view). */}
      {view === 'chat' && (
        <>
          {/* Mobile: dim backdrop behind the sessions drawer. */}
          {isMobile && mobileListOpen && (
            <div
              className="fixed inset-0 z-30 bg-black/50 md:hidden"
              onClick={() => setMobileListOpen(false)}
            />
          )}
          {/* Desktop: an always-visible column (md:static). Mobile: a left
              slide-in drawer toggled by the header hamburger. */}
          <div
            className={`shrink-0 md:static ${
              mobileListOpen ? 'max-md:translate-x-0' : 'max-md:-translate-x-full'
            } max-md:fixed max-md:inset-y-0 max-md:left-0 max-md:z-40 max-md:shadow-xl max-md:transition-transform`}
          >
            <SessionsSidebar
              sessions={ctl.sessions}
              agents={ctl.allAgents}
              activeSessionId={ctl.activeSessionId}
              streamingSessionIds={chat.streamingSessions}
              runtimeById={runtimeById}
              loading={ctl.bootstrapping}
              newDisabled={ctl.agents.length === 0}
              view={links.sessionListTab}
              onViewChange={links.setSessionListTab}
              kindFilter={links.sessionKindTab}
              onKindFilterChange={links.setSessionKindTab}
              totalSessions={ctl.sessionsTotal}
              hasMoreSessions={ctl.sessionsHasMore}
              onLoadMore={ctl.loadMoreSessions}
              onOpenOverview={() => setOverviewOpen(true)}
              onSelectSession={(id, messageId) => {
                ctl.selectSession(id, messageId)
                setMobileListOpen(false)
              }}
              onNewSession={() => {
                ctl.newSession()
                setMobileListOpen(false)
              }}
              onRefresh={ctl.refreshSessions}
              onRenameSession={ctl.renameSession}
              onGenerateTitle={ctl.regenerateSessionTitle}
              onCopyPath={ctl.copySessionPath}
              onRevealFolder={ctl.revealSession}
              onDeleteSession={ctl.deleteSession}
              onSetArchived={ctl.setSessionArchived}
              onSetPinned={ctl.setSessionPinned}
            />
          </div>
        </>
      )}

      {/* Key the view subtree by the active workspace so switching (or creating
          and switching into) a workspace REMOUNTS every panel. The panels here
          fetch their own workspace-scoped data on mount (flows, tasks, schedules,
          executions, network, artifacts, secrets, skills, market, budget,
          logs), so without a remount they would keep showing the previous
          workspace's data until a manual page refresh. App-level agents/sessions
          are reset+refetched by the activeWorkspaceId effect in the controller. */}
      <main
        key={activeWorkspaceId ?? 'none'}
        className="flex h-full min-w-0 flex-1 flex-col max-md:pb-[calc(3.25rem+env(safe-area-inset-bottom))]"
      >
        {!HEADERLESS_VIEWS.has(view) && (
          <AppHeader
            view={view}
            sessions={ctl.sessions}
            agents={ctl.agents}
            activeSessionId={ctl.activeSessionId}
            activeAgentId={ctl.activeAgentId}
            detailOpen={detailOpen}
            onOpenMobileList={() => setMobileListOpen(true)}
            navOpen={view === 'workspace' ? workspaceNav.open : settingsNav.open}
            onToggleNav={view === 'workspace' ? workspaceNav.toggle : settingsNav.toggle}
            onOpenContextPreview={() => setCtxPreviewOpen(true)}
            onOpenCoord={() => setCoordOpen(true)}
            onOpenDebug={() => setDebugOpen(true)}
            onOpenSessionFlow={() => setSessionFlowOpen((v) => !v)}
            sessionFlowActive={sessionFlowOpen}
            onToggleDetail={toggleDetail}
            onRevealSession={ctl.revealSession}
            onError={setError}
          />
        )}

        {view === 'chat' && sessionFlowOpen && ctl.activeSessionId && (
          <SessionFlowInline
            messages={ctl.messages}
            agents={ctl.agents}
            fallbackAgentId={ctl.activeAgentId || ctl.agents[0]?.id || ''}
            sessionId={ctl.activeSessionId}
            sessionTitle={ctl.sessions.find((s) => s.id === ctl.activeSessionId)?.title || ''}
            sessionKind={ctl.sessions.find((s) => s.id === ctl.activeSessionId)?.kind || ''}
            sourceId={ctl.sessions.find((s) => s.id === ctl.activeSessionId)?.sourceId}
            sessionCreatedAt={ctl.sessions.find((s) => s.id === ctl.activeSessionId)?.createdAt}
            onBack={() => setSessionFlowOpen(false)}
          />
        )}
        {view === 'chat' && !(sessionFlowOpen && ctl.activeSessionId) && (
          <ChatView
            chat={chat}
            messages={ctl.messages}
            agents={ctl.agents}
            artifacts={ctl.sessionArtifacts}
            activeSessionId={ctl.activeSessionId}
            activeAgentId={ctl.activeAgentId}
            bootstrapping={ctl.bootstrapping}
            messagesLoading={ctl.messagesLoading}
            readOnly={!ctl.activeSessionWritable}
            sessionCoordination={activeSession}
            onOpenRunHistory={openRunHistory}
            onSelectSession={ctl.selectSession}
            defaultAgentId={ctl.defaultAgentId}
            defaultAgentDeleted={ctl.defaultAgentDeleted}
            allAgents={ctl.allAgents}
            onNewSession={ctl.newSession}
            onSelectDefaultAgent={ctl.pickDefaultAgent}
            onGoToAgents={() => selectView('agents')}
            composerKey={ctl.composerKey}
            focusSessionId={ctl.focusSessionId}
            scrollToMsgId={ctl.scrollToMsgId}
            onHighlightConsumed={() => ctl.setScrollToMsgId(null)}
            onOpenFile={openFile}
            onOpenArtifact={links.openArtifact}
            onDeleteMessage={ctl.deleteMessage}
            onRewind={handleRewind}
            onFeedback={ctl.rateMessage}
            onOpenAgent={ctl.openAgentSettings}
            onAgentChange={ctl.changeChatAgent}
          />
        )}
        {view === 'agents' && (
          <AgentsView
            agents={ctl.agents}
            defaultAgentId={ctl.defaultAgentId}
            selectedId={ctl.activeAgentId}
            onSelectAgent={ctl.focusAgent}
            onSetDefault={ctl.pickAgent}
            onCreateAgent={ctl.createAgent}
            onUpdateAgent={ctl.updateAgent}
            onDuplicateAgent={ctl.duplicateAgent}
            onDeleteAgent={ctl.deleteAgent}
            onRefresh={() =>
              api
                .listAgents()
                .then(ctl.setAgents)
                .catch((e) => setError((e as Error).message))
            }
            onError={setError}
            onOpenExecution={(sid) => {
              // The agent activity rail links each run to its transcript; the
              // Executions screen is gone, so every kind opens in the unified
              // chat transcript view instead.
              setView('chat')
              ctl.selectSession(sid)
            }}
          />
        )}
        {view === 'network' && (
          <Suspense fallback={<LoadingState label="Ağ yükleniyor…" className="flex-1" />}>
            <NetworkPanel
              onError={setError}
              onOpenSession={(sid) => {
                setView('chat')
                ctl.selectSession(sid)
              }}
            />
          </Suspense>
        )}
        {view === 'explorer' && (
          <Suspense fallback={<LoadingState label="Harita yükleniyor…" className="flex-1" />}>
            <ExplorerView
              onError={setError}
              onOpenSession={(sid) => {
                setView('chat')
                ctl.selectSession(sid)
              }}
              focusNode={links.explorerNode}
              onFocusNode={links.setExplorerNode}
            />
          </Suspense>
        )}
        {view === 'board' && <TaskBoard agents={ctl.agents} onError={setError} />}
        {view === 'schedules' && (
          <AutomationBoard agents={ctl.agents} focusId={links.scheduleTarget} onError={setError} />
        )}
        {view === 'flows' && (
          <Suspense fallback={<LoadingState label="Akışlar yükleniyor…" className="flex-1" />}>
            <FlowsPanel
              agents={ctl.agents}
              onError={setError}
              openFlowId={links.flowTarget}
              tab={links.flowsTab}
              onTabChange={links.setFlowsTab}
            />
          </Suspense>
        )}
        {view === 'artifacts' && (
          <ArtifactsPanel
            onError={setError}
            agents={ctl.agents}
            selectedId={links.artifactTarget}
            onOpenSession={(sid) => {
              setView('chat')
              ctl.selectSession(sid)
            }}
          />
        )}
        {view === 'skills' && <SkillsPanel onError={setError} />}
        {view === 'tools' && <ToolCatalogPanel onError={setError} />}
        {view === 'market' && (
          <MarketPanel
            onError={setError}
            onManageSecrets={links.openSecrets}
            onInstalled={(kind) => {
              // Refresh the App-level agents list so a freshly installed agent
              // shows on the Agents screen without a manual reload. Flows/skills/
              // providers panels reload on their own mount.
              if (kind === 'agent')
                api
                  .listAgents()
                  .then(ctl.setAgents)
                  .catch(() => {})
            }}
          />
        )}
        {view === 'dashboard' && (
          <DashboardPanel
            onError={setError}
            nav={{
              openSession: (id) => {
                setView('chat')
                ctl.selectSession(id)
              },
              openView: (v) => setView(v),
            }}
          />
        )}
        {view === 'budget' && <BudgetPanel onError={setError} />}
        {view === 'logs' && <LogsPanel onError={setError} />}
        {view === 'insights' && (
          <InsightPanel
            onError={setError}
            tab={links.insightTab}
            onTabChange={links.setInsightTab}
            onOpenSession={(sid) => {
              setView('chat')
              ctl.selectSession(sid)
            }}
          />
        )}
        {view === 'workspace' && (
          <WorkspaceView
            onError={setError}
            onWorkspaceChanged={refreshWorkspaces}
            onDeleteWorkspace={deleteActiveWorkspace}
            onAppearanceSaved={onAppearanceSaved}
            onShowRecommendations={() => setRecsTrigger((n) => n + 1)}
            tab={links.workspaceTab}
            onTabChange={links.setWorkspaceTab}
            navOpen={workspaceNav.open}
            onToggleNav={workspaceNav.toggle}
          />
        )}
        {view === 'settings' && (
          <SettingsPanel
            onError={setError}
            onSaved={applyClientPrefs}
            onWorkspaceNotifySaved={onWorkspaceNotifySaved}
            commands={chat.chatCommands}
            cat={links.settingsCat}
            onCatChange={links.setSettingsCat}
            reloadNonce={settingsNonce}
            navOpen={settingsNav.open}
            onToggleNav={settingsNav.toggle}
          />
        )}
      </main>

      {view === 'chat' && detailOpen && ctl.activeSessionId && (
        <>
          {/* Mobile: dim backdrop behind the right detail drawer. */}
          {isMobile && (
            <div className="fixed inset-0 z-30 bg-black/50 md:hidden" onClick={toggleDetail} />
          )}
          {/* Desktop: a right-hand column. Mobile: a right slide-in drawer. */}
          <div className="shrink-0 md:static max-md:fixed max-md:inset-y-0 max-md:right-0 max-md:z-40 max-md:shadow-xl">
            <SessionDetailPanel
              sessionId={ctl.activeSessionId}
              refreshKey={ctl.meterRefresh}
              onClose={toggleDetail}
              onError={setError}
              onGenerateTitle={ctl.regenerateSessionTitle}
              onRename={ctl.renameSession}
              onDeleteSession={ctl.deleteSession}
              onSelectSession={ctl.selectSession}
              onRerun={() => chat.rerunLast()}
            />
          </div>
        </>
      )}

      {/* Coordination side sheet, opened from the chat header's "Coord" button. */}
      {view === 'chat' && coordOpen && ctl.activeSessionId && (
        <CoordinatorPanel
          sessionId={ctl.activeSessionId}
          refreshKey={ctl.meterRefresh}
          onClose={() => setCoordOpen(false)}
          onError={setError}
          onSelectSession={ctl.selectSession}
          onOpenSkill={openSkill}
        />
      )}

      {/* Bulk sessions overview: a searchable/sortable table of every session in
          the workspace, opened from the chat sidebar. Selecting a row jumps to
          that transcript in the chat view. */}
      {overviewOpen && (
        <SessionsOverview
          items={executions}
          agents={ctl.agents}
          onSelect={(sid) => {
            setView('chat')
            ctl.selectSession(sid)
          }}
          onClose={() => setOverviewOpen(false)}
        />
      )}

      {/* Bottom navigation for portrait phones (hidden on md+ where the rail
          shows). All views in one horizontally-scrollable strip. */}
      <MobileNavBar
        view={view}
        onSelectView={selectView}
        busyViews={busyViews}
        unreadViews={unreadViews}
        dirtyViews={dirtyViews as Set<View>}
        workspaces={workspaces}
        activeWorkspaceId={activeWorkspaceId}
        unreadWorkspaceIds={unreadWs}
        busyWorkspaceIds={busyWorkspaceIds}
        onSwitchWorkspace={switchWorkspace}
        onCreateWorkspace={handleCreateWorkspace}
      />

      {/* Next-turn context preview: opened from the chat header (works whether or
          not the detail inspector is open). */}
      {ctxPreviewOpen && ctl.activeSessionId && (
        <SessionContextModal
          sessionId={ctl.activeSessionId}
          title={ctl.sessions.find((s) => s.id === ctl.activeSessionId)?.title}
          updatedAt={ctl.sessions.find((s) => s.id === ctl.activeSessionId)?.updatedAt}
          onClose={() => setCtxPreviewOpen(false)}
        />
      )}

      {/* Debug / observability panel: opened from the chat header's "Debug" button
          (independent of the detail inspector). */}
      {debugOpen && ctl.activeSessionId && (
        <SessionDebugModal
          sessionId={ctl.activeSessionId}
          title={ctl.sessions.find((s) => s.id === ctl.activeSessionId)?.title}
          agentNames={Object.fromEntries(ctl.agents.map((a) => [a.id, a.name]))}
          onClose={() => setDebugOpen(false)}
        />
      )}

      {/* Artifact quick-preview overlay: opened by clicking an artifact card/chip
          in chat or the activity feed. Independent of the current view. */}
      {links.previewArtifactId && (
        <ArtifactPreviewModal
          artifactId={links.previewArtifactId}
          onClose={() => links.setPreviewArtifactId(null)}
          onOpenFull={links.openArtifactFull}
          onError={setError}
        />
      )}

      {/* App-wide transient-message surface (error / success / info). Every
          onError sink routes here via toast.error, so the headerless,
          sidebar-to-top screens still surface messages consistently. */}
      <Toaster />

      {/* ⌘K / Ctrl+K command palette: jump to any view or workspace. */}
      <CommandPalette
        open={paletteOpen}
        onClose={() => setPaletteOpen(false)}
        commands={paletteCommands}
      />

      {/* claude-cli gate: after a workspace is created/attached — and on every
          chat-screen entry when no provider is configured — probe the CLI login
          state. Offers the auth popup when the CLI is present but not logged in,
          or opens the Providers screen when the CLI is missing. */}
      <ClaudeAuthGate
        trigger={claudeGate.nonce}
        requireNoProvider={claudeGate.requireNoProvider}
        onNavigateProviders={() => {
          links.setSettingsCat('providers')
          setView('settings')
        }}
        onError={setError}
      />

      {/* Post-create advisory cards: after a new workspace is created/attached,
          offer one-click wiring for detected external tools (sqz hook,
          codebase-memory MCP) and steer toward setting a default working dir.
          Dismissible; never shown on chat-open. */}
      <WorkspaceRecommendations
        trigger={recsTrigger}
        onNavigateView={(v) => setView(v as View)}
        onNavigateSettings={(cat) => {
          links.setSettingsCat(cat)
          setView('settings')
        }}
        onError={setError}
      />
    </div>
  )
}
