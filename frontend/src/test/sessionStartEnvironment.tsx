import { createElement, type ReactNode } from 'react'
import { vi } from 'vitest'

const sessionStartTest = vi.hoisted(() => ({
  listAgents: vi.fn(),
  getWorkspaceSettings: vi.fn(),
  updateWorkspaceSettings: vi.fn(),
  listSessions: vi.fn(),
  getSessionsByIds: vi.fn(),
  createSession: vi.fn(),
  activeSessions: vi.fn(),
  listSessionArtifacts: vi.fn(),
  setSessionAgent: vi.fn(),
  listArtifacts: vi.fn(),
  noop: () => {},
  empty: [],
  emptySet: new Set(),
  emptyMap: new Map(),
  workspace: [{ id: 'WS1', name: 'Workspace' }],
}))

export { sessionStartTest }

vi.mock('@/api', () => ({ api: sessionStartTest, getActiveWorkspace: () => 'WS1' }))
vi.mock('@/app/useWorkspaces', () => ({
  useWorkspaces: () => ({
    workspaces: sessionStartTest.workspace,
    loading: false,
    activeWorkspaceId: 'WS1',
    unreadWs: sessionStartTest.emptySet,
    switchWorkspace: sessionStartTest.noop,
  }),
}))
vi.mock('@/app/useAppearance', () => ({ useAppearance: () => ({ notifyEnabled: false }) }))
vi.mock('@/app/useAppNavigation', () => ({
  INITIAL_ROUTE: { view: 'chat' },
  useAppNavigation: () => ({ applyRoute: sessionStartTest.noop }),
}))
vi.mock('@/app/useDeepLinks', () => ({ useDeepLinks: () => ({}) }))
vi.mock('@/app/useActivity', () => ({ useActivity: () => sessionStartTest.emptySet }))
vi.mock('@/app/useWorkspaceActivity', () => ({
  useWorkspaceActivity: () => sessionStartTest.emptySet,
}))
vi.mock('@/app/useUnreadViews', () => ({
  useUnreadViews: () => ({
    unreadViews: sessionStartTest.emptySet,
    markViewRead: sessionStartTest.noop,
  }),
}))
vi.mock('@/app/useUnreadBadge', () => ({ useUnreadBadge: sessionStartTest.noop }))
vi.mock('@/app/useAppEvents', () => ({ useAppEvents: sessionStartTest.noop }))
vi.mock('@/app/useWorkspaceSignals', () => ({ useWorkspaceSignals: sessionStartTest.noop }))
vi.mock('@/app/useExecutionRuntime', () => ({
  useExecutionRuntime: () => ({ runtimeById: sessionStartTest.emptyMap }),
}))
vi.mock('@/shared/lib/dirtySignals', () => ({
  useDirtyViews: () => sessionStartTest.emptySet,
  useRegisterDirty: sessionStartTest.noop,
}))
vi.mock('@/shared/hooks/useMediaQuery', () => ({ useIsMobile: () => false }))
vi.mock('@/shared/hooks/useViewport', () => ({ useViewportAttribute: sessionStartTest.noop }))
vi.mock('@/shared/hooks/useCollapsibleList', () => ({
  useCollapsibleList: () => ({
    open: true,
    toggle: sessionStartTest.noop,
    setOpen: sessionStartTest.noop,
  }),
}))
vi.mock('@/app/useShellLayout', () => ({ useShellLayout: () => ({ list: 'docked' }) }))
vi.mock('@/features/sessions/useSessionChips', () => ({
  useSessionChips: () => ({
    chipsParam: '',
    chipsOff: sessionStartTest.empty,
    chipSet: sessionStartTest.emptySet,
    clickChip: sessionStartTest.noop,
  }),
}))
vi.mock('@/app/useTranscript', () => ({
  useTranscript: () => ({
    messages: sessionStartTest.empty,
    setMessages: sessionStartTest.noop,
    messagesLoading: false,
    refreshMessages: sessionStartTest.noop,
  }),
}))
vi.mock('@/shared/hooks/useReferencedAgents', () => ({
  useReferencedAgents: (agents: unknown) => agents,
}))
vi.mock('@/shared/lib/tts', () => ({
  initServerTts: sessionStartTest.noop,
  initTtsUnlock: sessionStartTest.noop,
}))
vi.mock('@/shared/lib/stt', () => ({ initServerStt: sessionStartTest.noop }))
vi.mock('@/features/chat/useChatStream', () => ({
  useChatStream: () => ({
    streamingSessions: sessionStartTest.emptySet,
    activeQueued: sessionStartTest.empty,
    reconcileActive: sessionStartTest.noop,
    markPending: sessionStartTest.noop,
  }),
}))
vi.mock('@/features/chat/useRunningWorkers', () => ({
  useRunningWorkers: () => sessionStartTest.empty,
}))
vi.mock('@/app/NavRail', () => ({ NavRail: () => null }))
vi.mock('@/app/MobileNavBar', () => ({ MobileNavBar: () => null }))
vi.mock('@/app/AppHeader', () => ({ AppHeader: () => null }))
vi.mock('@/app/UpdateBanner', () => ({ UpdateBanner: () => null }))
vi.mock('@/app/ConnectionNotice', () => ({ ConnectionNotice: () => null }))
vi.mock('@/features/workspace/ClaudeAuthGate', () => ({ ClaudeAuthGate: () => null }))
vi.mock('@/features/workspace/WorkspaceRecommendations', () => ({
  WorkspaceRecommendations: () => null,
}))
vi.mock('@/features/sessions/SessionsSidebar', () => ({
  SessionsSidebar: ({
    newDisabled,
    onNewSession,
  }: {
    newDisabled: boolean
    onNewSession: () => void
  }) =>
    createElement(
      'button',
      { disabled: newDisabled, onClick: onNewSession, 'data-testid': 'sidebar-new-chat' },
      'New chat',
    ),
}))
vi.mock('@/shared/components', async () => ({
  ...(await import('@/test/formStubs')),
  CollapsibleListShell: ({ children }: { children: ReactNode }) => children,
  Backdrop: () => null,
  Toaster: () => null,
  CommandPalette: () => null,
  LoadingState: () => null,
  toast: { error: vi.fn() },
}))
vi.mock('@/app/lazyPanels', () => ({}))
vi.mock('@/app/lazyChatPanels', async () => ({
  ChatView: (await import('@/features/chat/ChatView')).ChatView,
  SessionDetailPanel: () => null,
  CoordinatorPanel: () => null,
  SessionContextModal: () => null,
  SessionDebugModal: () => null,
  ArtifactPreviewModal: () => null,
}))
