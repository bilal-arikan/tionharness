import { describe, expect, it, vi } from 'vitest'
import type { Agent, InsightSettings } from '@/types'
import { persistAnalysisAgentSelection, withDefaultAnalysisAgent } from './insightAgentSelection'
import panelSource from './InsightPanel.tsx?raw'
import settingsSource from './SettingsTab.tsx?raw'

const agents = [{ id: 'AGT1' }, { id: 'AGT2' }] as Agent[]

describe('InsightPanel analysis agent selection', () => {
  it('defaults an empty selection to the first loaded agent', () => {
    expect(withDefaultAnalysisAgent({}, agents)).toEqual({ autoScanAgentId: 'AGT1' })
    expect(withDefaultAnalysisAgent({}, [])).toEqual({})
    expect(withDefaultAnalysisAgent({ autoScanAgentId: 'AGT2' }, agents)).toEqual({
      autoScanAgentId: 'AGT2',
    })
  })

  it('optimistically selects and persists the complete settings payload', async () => {
    const settings: InsightSettings = { autoScanAgentId: 'AGT1', maxSessions: 25 }
    const setSettings = vi.fn()
    const updateSettings = vi.fn(async (next: InsightSettings) => next)

    await persistAnalysisAgentSelection(settings, 'AGT2', setSettings, updateSettings, vi.fn())

    const expected = { autoScanAgentId: 'AGT2', maxSessions: 25 }
    expect(updateSettings).toHaveBeenCalledWith(expected)
    expect(setSettings).toHaveBeenNthCalledWith(1, expected)
    expect(setSettings).toHaveBeenLastCalledWith(expected)
  })

  it('reports persistence errors and rolls the selection back', async () => {
    const settings: InsightSettings = { autoScanAgentId: 'AGT1', maxSessions: 25 }
    const setSettings = vi.fn()
    const onError = vi.fn()

    await persistAnalysisAgentSelection(
      settings,
      'AGT2',
      setSettings,
      async () => {
        throw new Error('PUT failed')
      },
      onError,
    )

    expect(setSettings).toHaveBeenLastCalledWith(settings)
    expect(onError).toHaveBeenCalledWith('PUT failed')
  })

  it('keeps the picker under Tara in the rail and out of SettingsTab', () => {
    expect(panelSource).toContain('Promise.all([api.getInsightSettings(), api.listAgents()])')
    expect(panelSource).toContain('withDefaultAnalysisAgent(loadedSettings, loadedAgents)')
    expect(panelSource.indexOf('Retrospektif tarama başlat')).toBeGreaterThan(-1)
    expect(panelSource.indexOf('Analiz ajanı')).toBeGreaterThan(
      panelSource.indexOf('Retrospektif tarama başlat'),
    )
    expect(panelSource.indexOf('Analiz ajanı')).toBeLessThan(panelSource.indexOf('Sub-page rail.'))
    expect(settingsSource).not.toContain('AgentPicker')
    expect(settingsSource).not.toContain('Analiz ajanı')
  })

  it('keeps manual scans on the persisted backend agent fallback', () => {
    expect(panelSource).toContain('api.runInsightScan(lensIds ? { lensIds } : {})')
    expect(panelSource).not.toContain('analysisAgentId')
  })
})
