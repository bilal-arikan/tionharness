// useDeepLinks owns the per-view deep-link targets (the entity a routed screen
// should pre-select) plus the cross-view "open X" helpers that set a target and
// switch the view in one step.
import { useCallback, useEffect, useState } from 'react'
import type { View } from './NavRail'
import { INITIAL_ROUTE } from './useAppNavigation'
import {
  KIND_FILTER_KEY,
  normalizeKindFilter,
  normalizeSessionListTab,
  type SessionListTab,
} from '@/features/sessions/sessionKindMeta'

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
  // Selected Explorer map node (deep-link aware): #/w/{ws}/explorer/{refString}.
  const [explorerNode, setExplorerNode] = useState<string | null>(
    INITIAL_ROUTE.view === 'explorer' ? INITIAL_ROUTE.id : null,
  )
  // Active Flows sub-tab (deep-link aware): #/w/{ws}/flows/{tab} (flows|templates|runs).
  const [flowsTab, setFlowsTab] = useState<string | null>(
    INITIAL_ROUTE.view === 'flows' ? INITIAL_ROUTE.id : null,
  )
  // Sessions sidebar tabs (deep-link aware): #/w/{ws}/chat/{sessionId}?list=…&kind=…
  // They live here (not inside the sidebar) so the URL can address them; the chat
  // view's single entity slot is already spent on the session id.
  const [sessionListTab, setSessionListTab] = useState<SessionListTab>(() =>
    normalizeSessionListTab(INITIAL_ROUTE.view === 'chat' ? INITIAL_ROUTE.query?.list : null),
  )
  // The kind filter is also persisted, so a plain "#/…/chat" load restores the
  // last tab; an explicit ?kind= in the URL takes precedence over storage.
  const [sessionKindTab, setSessionKindTab] = useState<string>(() => {
    const fromUrl = INITIAL_ROUTE.view === 'chat' ? INITIAL_ROUTE.query?.kind : undefined
    return normalizeKindFilter(fromUrl ?? localStorage.getItem(KIND_FILTER_KEY))
  })
  useEffect(() => {
    localStorage.setItem(KIND_FILTER_KEY, sessionKindTab)
  }, [sessionKindTab])

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
    sessionListTab,
    setSessionListTab,
    sessionKindTab,
    setSessionKindTab,
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
