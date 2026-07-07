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
  // Artifact deep-link target: set when a chat artifact card is clicked, opening
  // the artifacts screen with that artifact pre-selected.
  const [artifactTarget, setArtifactTarget] = useState<string | null>(
    INITIAL_ROUTE.view === 'artifacts' ? INITIAL_ROUTE.id : null,
  )
  // Flow deep-link target: set when an Activity flow execution links to its flow,
  // opening the Flows screen on that flow's run history.
  const [flowTarget, setFlowTarget] = useState<string | null>(null)
  const openFlowRun = useCallback((flowId: string) => {
    setFlowTarget(flowId)
    setView('flows')
  }, [setView])
  // Executions deep-link target (sessionId): set when a schedule/flow notification
  // is clicked, opening the Activity feed with that run pre-selected.
  const [executionTarget, setExecutionTarget] = useState<string | null>(
    INITIAL_ROUTE.view === 'executions' ? INITIAL_ROUTE.id : null,
  )
  // Open a specific run on the Activity screen (used by the agent activity rail).
  const openExecution = useCallback((sessionId: string) => {
    setExecutionTarget(sessionId)
    setView('executions')
  }, [setView])

  // Clicking an artifact card/chip anywhere (chat, activity): preview it in a
  // modal overlay — no navigation to the Artifacts screen. The modal offers a
  // shortcut to open the full screen for editing.
  const [previewArtifactId, setPreviewArtifactId] = useState<string | null>(null)
  const openArtifact = useCallback((id: string) => {
    setPreviewArtifactId(id)
  }, [])
  // Open the dedicated Artifacts screen on a specific artifact (from the preview
  // modal's "open in screen" shortcut).
  const openArtifactFull = useCallback((id: string) => {
    setPreviewArtifactId(null)
    setArtifactTarget(id)
    setView('artifacts')
  }, [setView])

  // Secrets moved under Settings as a sub-category: open the Settings screen
  // focused on the Secrets ("Sırlar") category.
  const openSecrets = useCallback(() => {
    setSettingsCat('secrets')
    setView('settings')
  }, [setView])

  return {
    scheduleTarget, setScheduleTarget,
    settingsCat, setSettingsCat,
    workspaceTab, setWorkspaceTab,
    artifactTarget, setArtifactTarget,
    flowTarget, openFlowRun,
    executionTarget, setExecutionTarget, openExecution,
    previewArtifactId, setPreviewArtifactId, openArtifact, openArtifactFull,
    openSecrets,
  }
}
