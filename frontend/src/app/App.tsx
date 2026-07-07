// App is the composition root: it wires the app-level hooks (workspaces,
// appearance, sessions controller, chat stream, SSE events, URL routing) into
// the shell layout (nav rail, per-view panels, modals). The per-concern logic
// lives in the app/use*.ts hooks; the view metadata in viewRegistry.tsx.
import { useCallback, useEffect, useMemo, useState, Suspense } from 'react'
import { api } from '@/api'
import { ErrorToast } from '@/shared/components/ErrorToast'
import { NavRail, type View } from './NavRail'
import { MobileNavBar } from './MobileNavBar'
import { SplashScreen } from './SplashScreen'
import { AppHeader } from './AppHeader'
import {
  FlowsPanel,
  NetworkPanel,
  HEADERLESS_VIEWS,
  SPLASH_MIN_MS,
} from './viewRegistry'
import { INITIAL_ROUTE, useAppNavigation } from './useAppNavigation'
import { useAppearance } from './useAppearance'
import { useAppEvents } from './useAppEvents'
import { useDeepLinks } from './useDeepLinks'
import { useSessionsController } from './useSessionsController'
import { useWorkspaces } from './useWorkspaces'
import { useActivity } from './useActivity'
import { useUnreadViews } from './useUnreadViews'
import { useUnreadBadge } from './useUnreadBadge'
import { ChatView } from '@/features/chat/ChatView'
import { useChatStream } from '@/features/chat/useChatStream'
import { writeSessionDraft } from '@/features/chat/useSessionDraft'
import { SessionsSidebar } from '@/features/sessions/SessionsSidebar'
import { SessionDetailPanel } from '@/features/sessions/SessionDetailPanel'
import { SessionContextModal } from '@/features/sessions/SessionContextModal'
import { SessionDebugModal } from '@/features/sessions/SessionDebugModal'
import { AgentsView } from '@/features/agents/AgentsView'
import { ExecutionsPanel } from '@/features/executions/ExecutionsPanel'
import { TaskBoard } from '@/features/tasks/TaskBoard'
import { Schedules } from '@/features/schedules/Schedules'
import { ArtifactsPanel } from '@/features/artifacts/ArtifactsPanel'
import { ArtifactPreviewModal } from '@/features/artifacts/ArtifactPreviewModal'
import { SkillsPanel } from '@/features/skills/SkillsPanel'
import { ToolsPanel as ToolCatalogPanel } from '@/features/tools/ToolsPanel'
import { MarketPanel } from '@/features/market/MarketPanel'
import { BudgetPanel } from '@/features/budget/BudgetPanel'
import { LogsPanel } from '@/features/logs/LogsPanel'
import { SettingsPanel } from '@/features/settings/SettingsPanel'
import { WorkspaceView } from '@/features/workspace/WorkspaceView'
import { OnboardingScreen } from '@/features/workspace/OnboardingScreen'
import { ClaudeAuthGate } from '@/features/workspace/ClaudeAuthGate'
import { useDirtyViews } from '@/shared/lib/dirtySignals'
import { useIsMobile } from '@/shared/hooks/useMediaQuery'
import { useCollapsibleList } from '@/shared/hooks/useCollapsibleList'
import { isImagePath, mediaUrl } from '@/shared/lib/paths'
import { copyToClipboard } from '@/shared/lib/clipboard'

export default function App() {
  const [error, setError] = useState<string | null>(null)
  const [view, setView] = useState<View>(INITIAL_ROUTE.view)
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

  // Post-create claude-cli readiness gate: bumped once per successful workspace
  // creation so ClaudeAuthGate (re-)probes the new workspace's login state and
  // either offers the auth popup or steers to the Providers screen.
  const [claudeGateNonce, setClaudeGateNonce] = useState(0)
  const handleCreateWorkspace = useCallback(
    async (data: Parameters<typeof createWorkspace>[0]) => {
      const created = await createWorkspace(data)
      if (created) setClaudeGateNonce((n) => n + 1)
    },
    [createWorkspace],
  )

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

  // Theme / keep-awake / desktop-notification preferences.
  const { applyClientPrefs, onAppearanceSaved, notifyEnabled } = useAppearance(
    activeWorkspaceId,
    setError,
  )

  // Per-view deep-link targets + cross-view "open X" helpers.
  const links = useDeepLinks(setView)

  // Workspace-scoped data model: agents, sessions, transcript + their actions.
  const ctl = useSessionsController({
    activeWorkspaceId,
    setError,
    setView,
    setExecutionTarget: links.setExecutionTarget,
  })

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
    setView,
    selectSession: ctl.selectSession,
    refreshSessions: ctl.refreshSessions,
    bumpMeter: ctl.bumpMeter,
  })
  // Publish the live chat handle for the controller's messages-load effect.
  ctl.chatRef.current = chat

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

  // After a reload (or workspace switch), restore the "thinking" indicator for
  // any turn still running detached on the server — the user may have refreshed
  // right after sending. Each turn's chat-completion event clears it again.
  useEffect(() => {
    if (!activeWorkspaceId) return
    api.activeSessions().then(chat.markPending).catch(() => {})
  }, [activeWorkspaceId, chat.markPending])

  // Per-view "work in progress" flags for the nav-rail busy indicators.
  const busyViews = useActivity(activeWorkspaceId, chat.streamingSessions.size > 0)
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
    const sess = ctl.chatSessions.filter((s) => s.unread).length
    const views = [...unreadViews].filter((v) => v !== 'chat').length
    return sess + unreadWs.size + views
  }, [ctl.chatSessions, unreadViews, unreadWs])
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
    executionTarget: links.executionTarget,
    pendingRouteRef: ctl.pendingRouteRef,
    switchWorkspace,
    selectSession: ctl.selectSession,
    focusAgent: ctl.focusAgent,
    setArtifactTarget: links.setArtifactTarget,
    setScheduleTarget: links.setScheduleTarget,
    setExecutionTarget: links.setExecutionTarget,
    setSettingsCat: links.setSettingsCat,
    setWorkspaceTab: links.setWorkspaceTab,
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
    return <OnboardingScreen onCreate={handleCreateWorkspace} onAttach={attachWorkspace} />
  }

  return (
    <div className="flex h-full">
      <NavRail
        view={view}
        onSelectView={selectView}
        workspaces={workspaces}
        activeWorkspaceId={activeWorkspaceId}
        unreadWorkspaceIds={unreadWs}
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
              sessions={ctl.chatSessions}
              agents={ctl.agents}
              activeSessionId={ctl.activeSessionId}
              streamingSessionIds={chat.streamingSessions}
              newDisabled={ctl.agents.length === 0}
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
            onOpenDebug={() => setDebugOpen(true)}
            onToggleDetail={toggleDetail}
            onRevealSession={ctl.revealSession}
            onError={setError}
          />
        )}

        {view === 'chat' && (
          <ChatView
            chat={chat}
            messages={ctl.messages}
            agents={ctl.agents}
            artifacts={ctl.sessionArtifacts}
            activeSessionId={ctl.activeSessionId}
            activeAgentId={ctl.activeAgentId}
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
            onDeleteAgent={ctl.deleteAgent}
            onRefresh={() =>
              api
                .listAgents()
                .then(ctl.setAgents)
                .catch((e) => setError((e as Error).message))
            }
            onError={setError}
            onOpenExecution={links.openExecution}
          />
        )}
        {view === 'executions' && (
          <ExecutionsPanel
            agents={ctl.agents}
            onError={setError}
            onOpenFile={openFile}
            onOpenArtifact={links.openArtifact}
            onOpenFlowRun={links.openFlowRun}
            focusId={links.executionTarget}
            onSelectExecution={links.setExecutionTarget}
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
        {view === 'board' && <TaskBoard agents={ctl.agents} onError={setError} />}
        {view === 'schedules' && (
          <Schedules agents={ctl.agents} focusId={links.scheduleTarget} onError={setError} />
        )}
        {view === 'flows' && (
          <Suspense
            fallback={
              <div className="flex-1 p-6 text-sm text-[var(--color-text-dim)]">Akışlar yükleniyor…</div>
            }
          >
            <FlowsPanel agents={ctl.agents} onError={setError} openFlowId={links.flowTarget} />
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
              if (kind === 'agent') api.listAgents().then(ctl.setAgents).catch(() => {})
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
            <div
              className="fixed inset-0 z-30 bg-black/50 md:hidden"
              onClick={toggleDetail}
            />
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
        onSwitchWorkspace={switchWorkspace}
        onCreateWorkspace={handleCreateWorkspace}
      />

      {/* Next-turn context preview: opened from the chat header (works whether or
          not the detail inspector is open). */}
      {ctxPreviewOpen && ctl.activeSessionId && (
        <SessionContextModal
          sessionId={ctl.activeSessionId}
          title={ctl.sessions.find((s) => s.id === ctl.activeSessionId)?.title}
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

      {/* App-wide error surface (replaces the per-view header error span so the
          headerless, sidebar-to-top screens still show errors consistently). */}
      <ErrorToast message={error ?? ''} onDismiss={() => setError(null)} />

      {/* Post-create claude-cli gate: after a new workspace is created, probe its
          login state — offer the auth popup when the CLI is present but not
          logged in, or open the Providers screen when the CLI is missing. */}
      <ClaudeAuthGate
        trigger={claudeGateNonce}
        onNavigateProviders={() => {
          links.setSettingsCat('providers')
          setView('settings')
        }}
        onError={setError}
      />
    </div>
  )
}
