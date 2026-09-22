import type { Agent, InsightSettings } from '@/types'
import { pickableAgents } from '@/shared/components/agents/pickableAgents'

export function withDefaultAnalysisAgent(
  settings: InsightSettings,
  agents: Agent[],
): InsightSettings {
  // Never default to an archived agent: it cannot run the analysis.
  const live = pickableAgents(agents)
  if (settings.autoScanAgentId || live.length === 0) return settings
  return { ...settings, autoScanAgentId: live[0].id }
}

export async function persistAnalysisAgentSelection(
  settings: InsightSettings,
  agentId: string,
  setSettings: (settings: InsightSettings) => void,
  updateSettings: (settings: InsightSettings) => Promise<InsightSettings>,
  onError: (message: string) => void,
) {
  const next = { ...settings, autoScanAgentId: agentId }
  setSettings(next)
  try {
    setSettings(await updateSettings(next))
  } catch (error) {
    setSettings(settings)
    onError((error as Error).message)
  }
}
