import { lazy } from 'react'

export const ChatView = lazy(() =>
  import('@/features/chat/ChatView').then((m) => ({ default: m.ChatView })),
)
export const SessionDetailPanel = lazy(() =>
  import('@/features/sessions/SessionDetailPanel').then((m) => ({ default: m.SessionDetailPanel })),
)
export const CoordinatorPanel = lazy(() =>
  import('@/features/sessions/CoordinatorPanel').then((m) => ({ default: m.CoordinatorPanel })),
)
export const SessionContextModal = lazy(() =>
  import('@/features/sessions/SessionContextModal').then((m) => ({
    default: m.SessionContextModal,
  })),
)
export const SessionDebugModal = lazy(() =>
  import('@/features/sessions/SessionDebugModal').then((m) => ({ default: m.SessionDebugModal })),
)
export const ArtifactPreviewModal = lazy(() =>
  import('@/features/artifacts/ArtifactPreviewModal').then((m) => ({
    default: m.ArtifactPreviewModal,
  })),
)
