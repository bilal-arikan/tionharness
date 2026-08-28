import type { Agent, InsightSettings } from '@/types'

export function withDefaultAnalysisAgent(
  settings: InsightSettings,
  agents: Agent[],
): InsightSettings {
  if (settings.autoScanAgentId || agents.length === 0) return settings
  return { ...settings, autoScanAgentId: agents[0].id }
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
