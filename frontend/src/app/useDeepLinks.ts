// useDeepLinks owns the per-view deep-link targets (the entity a routed screen
// should pre-select) plus the cross-view "open X" helpers that set a target and
// switch the view in one step.
import { useCallback, useState } from 'react'
import type { View } from './NavRail'
import { INITIAL_ROUTE } from './useAppNavigation'

export function useDeepLinks(setView: (v: View) => void) {
  // Deep-link target for the schedules screen (highlights the routed schedule).
  const [scheduleTarget, setScheduleTarget] = useState<string | null>(
    INITIAL_ROUTE.view === 'schedules' ? INITIAL_ROUTE.id : null,
  )
  // Active settings category (deep-link aware): #/w/{ws}/settings/{category}.
  const [settingsCat, setSettingsCat] = useState<string | null>(
    INITIAL_ROUTE.view === 'settings' ? INITIAL_ROUTE.id : null,
  )
  // Active workspace sub-tab (deep-link aware): #/w/{ws}/workspace/{tab}.
  const [workspaceTab, setWorkspaceTab] = useState<string | null>(
    INITIAL_ROUTE.view === 'workspace' ? INITIAL_ROUTE.id : null,
  )
  // Active İçgörü sub-tab (deep-link aware): #/w/{ws}/insights/{tab}.
  const [insightTab, setInsightTab] = useState<string | null>(
    INITIAL_ROUTE.view === 'insights' ? INITIAL_ROUTE.id : null,
  )
  // Explorer focus node (selection remains local): #/w/{ws}/explorer/{refString}.
  const [explorerNode, setExplorerNode] = useState<string | null>(
    INITIAL_ROUTE.view === 'explorer' ? INITIAL_ROUTE.id : null,
  )
  // Active Flows sub-tab (deep-link aware): #/w/{ws}/flows/{tab} (flows|templates|runs).
  const [flowsTab, setFlowsTab] = useState<string | null>(
    INITIAL_ROUTE.view === 'flows' ? INITIAL_ROUTE.id : null,
  )
  // Task card to open on the board (deep-link aware): #/w/{ws}/board/{taskId}.
  const [boardTarget, setBoardTarget] = useState<string | null>(
    INITIAL_ROUTE.view === 'board' ? INITIAL_ROUTE.id : null,
  )
  // Selected tool group on the Tools screen (deep-link aware): #/w/{ws}/tools/{group}.
  const [toolsGroup, setToolsGroup] = useState<string | null>(
    INITIAL_ROUTE.view === 'tools' ? INITIAL_ROUTE.id : null,
  )
  // Zoomed trajectory on the Rota screen (deep-link aware): #/w/{ws}/rota/{RTA}.
  const [rotaTrajectory, setRotaTrajectory] = useState<string | null>(
    INITIAL_ROUTE.view === 'rota' ? INITIAL_ROUTE.id : null,
  )
  // Open the Rota screen zoomed on one trajectory (chat header strip, session
  // panel, run view).
  const openTrajectory = useCallback(
    (id: string) => {
      setRotaTrajectory(id)
      setView('rota')
    },
    [setView],
  )

  // Artifact deep-link target: set when a chat artifact card is clicked, opening
  // the artifacts screen with that artifact pre-selected.
  const [artifactTarget, setArtifactTarget] = useState<string | null>(
    INITIAL_ROUTE.view === 'artifacts' ? INITIAL_ROUTE.id : null,
  )
  // Flow deep-link target: set when a flow transcript links to its flow, opening
  // the Flows screen on that flow's run history.
  const [flowTarget, setFlowTarget] = useState<string | null>(null)
  const openFlowRun = useCallback(
    (flowId: string) => {
      setFlowTarget(flowId)
      setView('flows')
    },
    [setView],
  )

  // Clicking an artifact card/chip anywhere: preview it in a
  // modal overlay — no navigation to the Artifacts screen. The modal offers a
  // shortcut to open the full screen for editing.
  const [previewArtifactId, setPreviewArtifactId] = useState<string | null>(null)
  const openArtifact = useCallback((id: string) => {
    setPreviewArtifactId(id)
  }, [])
  // Open the dedicated Artifacts screen on a specific artifact (from the preview
  // modal's "open in screen" shortcut).
  const openArtifactFull = useCallback(
    (id: string) => {
      setPreviewArtifactId(null)
      setArtifactTarget(id)
      setView('artifacts')
    },
    [setView],
  )

  // Secrets moved under Settings as a sub-category: open the Settings screen
  // focused on the Secrets ("Sırlar") category.
  const openSecrets = useCallback(() => {
    setSettingsCat('secrets')
    setView('settings')
  }, [setView])

  return {
    scheduleTarget,
    setScheduleTarget,
    settingsCat,
    setSettingsCat,
    workspaceTab,
    setWorkspaceTab,
    insightTab,
    setInsightTab,
    explorerNode,
    setExplorerNode,
    flowsTab,
    setFlowsTab,
    rotaTrajectory,
    setRotaTrajectory,
    boardTarget,
    setBoardTarget,
    toolsGroup,
    setToolsGroup,
    openTrajectory,
    artifactTarget,
    setArtifactTarget,
    flowTarget,
    openFlowRun,
    previewArtifactId,
    setPreviewArtifactId,
    openArtifact,
    openArtifactFull,
    openSecrets,
  }
}
