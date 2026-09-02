// useAppNavigation owns the URL ↔ state machinery: the canonical route for the
// current app state (mirrored to the hash by useUrlSync) and applyRoute, which
// turns a parsed Route (back/forward, manual URL edit, shared link, agent
// focus_view navigation) back into app state.
import { useCallback, type MutableRefObject } from 'react'
import { setActiveWorkspace, getActiveWorkspace } from '@/api'
import { parseRoute, routeIdForView, type Route } from './url'
import { useUrlSync } from './useUrlSync'
import type { View } from './NavRail'

// Parse the deep-link once at module load. If it names a workspace, apply it to
// the api client immediately so useWorkspaces initialises on the routed
// workspace (an unknown id is validated away to the first workspace there).
export const INITIAL_ROUTE: Route = parseRoute(window.location.hash)
if (INITIAL_ROUTE.workspaceId) setActiveWorkspace(INITIAL_ROUTE.workspaceId)

export interface AppNavigationParams {
  view: View
  setView: (v: View) => void
  activeWorkspaceId: string | null
  activeSessionId: string | null
  activeAgentId: string | null
  artifactTarget: string | null
  scheduleTarget: string | null
  settingsCat: string | null
  workspaceTab: string | null
  insightTab: string | null
  explorerNode: string | null
  flowsTab: string | null
  rotaTrajectory: string | null
  pendingRouteRef: MutableRefObject<Route | null>
  switchWorkspace: (id: string) => void
  selectSession: (id: string, messageId?: string) => void
  focusAgent: (id: string) => void
  setArtifactTarget: (id: string | null) => void
  setScheduleTarget: (id: string | null) => void
  setSettingsCat: (id: string | null) => void
  setWorkspaceTab: (id: string | null) => void
  setInsightTab: (id: string | null) => void
  setExplorerNode: (id: string | null) => void
  setFlowsTab: (id: string | null) => void
  setRotaTrajectory: (id: string | null) => void
}

export function useAppNavigation(p: AppNavigationParams) {
  const {
    setView,
    pendingRouteRef,
    switchWorkspace,
    selectSession,
    focusAgent,
    setArtifactTarget,
    setScheduleTarget,
    setSettingsCat,
    setWorkspaceTab,
    setInsightTab,
    setExplorerNode,
    setFlowsTab,
    setRotaTrajectory,
  } = p

  // Apply a Route (from back/forward, a manual URL edit, or a shared link) to
  // the app state. A workspace switch defers entity selection to the
  // workspace-load effect via pendingRouteRef; same-workspace navigation applies
  // the entity immediately.
  const applyRoute = useCallback(
    (r: Route) => {
      setView(r.view)
      if (r.workspaceId && r.workspaceId !== getActiveWorkspace()) {
        pendingRouteRef.current = r
        switchWorkspace(r.workspaceId)
        return
      }
      if (r.view === 'chat') {
        if (r.id) selectSession(r.id)
      } else if (r.view === 'agents') {
        if (r.id) focusAgent(r.id)
      } else if (r.view === 'artifacts') {
        setArtifactTarget(r.id)
      } else if (r.view === 'schedules') {
        setScheduleTarget(r.id)
      } else if (r.view === 'settings') {
        setSettingsCat(r.id)
      } else if (r.view === 'workspace') {
        setWorkspaceTab(r.id)
      } else if (r.view === 'insights') {
        setInsightTab(r.id)
      } else if (r.view === 'explorer') {
        setExplorerNode(r.id)
      } else if (r.view === 'flows') {
        setFlowsTab(r.id)
      } else if (r.view === 'rota') {
        setRotaTrajectory(r.id)
      }
    },
    [
      setView,
      pendingRouteRef,
      switchWorkspace,
      selectSession,
      focusAgent,
      setArtifactTarget,
      setScheduleTarget,
      setSettingsCat,
      setWorkspaceTab,
      setInsightTab,
      setExplorerNode,
      setFlowsTab,
      setRotaTrajectory,
    ],
  )

  // The canonical route for the current state, mirrored to the URL hash.
  const route: Route = {
    workspaceId: p.activeWorkspaceId,
    view: p.view,
    id: routeIdForView(p.view, {
      sessionId: p.activeSessionId,
      agentId: p.activeAgentId,
      artifactId: p.artifactTarget,
      scheduleId: p.scheduleTarget,
      settingsCat: p.settingsCat,
      workspaceTab: p.workspaceTab,
      insightTab: p.insightTab,
      flowsTab: p.flowsTab,
      explorerNode: p.explorerNode,
      rotaTrajectory: p.rotaTrajectory,
    }),
  }
  useUrlSync(route, !!p.activeWorkspaceId, applyRoute)
}
