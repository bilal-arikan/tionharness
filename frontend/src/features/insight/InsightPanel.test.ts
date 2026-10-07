// @vitest-environment jsdom

import { act, createElement, type ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Agent, InsightSettings } from '@/types'
import { render, query } from '@/test/render'
import { persistAnalysisAgentSelection, withDefaultAnalysisAgent } from './insightAgentSelection'
import { InsightPanel } from './InsightPanel'

const calls = vi.hoisted(() => ({
  getInsightSettings: vi.fn(),
  listAgents: vi.fn(),
  listInsightLenses: vi.fn(),
  listInsightFindings: vi.fn(),
  getInsightScanStatus: vi.fn(),
  updateInsightSettings: vi.fn(),
  runInsightScan: vi.fn(),
  t: (key: string) => key,
}))

vi.mock('@/api', () => ({ api: calls }))
vi.mock('react-i18next', async (importOriginal) => ({
  ...(await importOriginal<typeof import('react-i18next')>()),
  useTranslation: () => ({ t: calls.t }),
}))
vi.mock('@/shared/components', () => ({
  ListPane: ({ children }: { children: ReactNode }) =>
    createElement('aside', { 'data-testid': 'scan-rail' }, children),
  PaneHeader: () => null,
  InfoPopover: () => null,
  toast: { success: vi.fn() },
}))
vi.mock('@/shared/components/SidebarChrome', () => ({ SidebarHeader: () => null }))
vi.mock('@/shared/hooks/useCollapsibleList', () => ({
  useCollapsibleList: () => ({ open: true, toggle: vi.fn() }),
}))
vi.mock('@/shared/components/agents/AgentIdentity', () => ({
  AgentIdentity: ({ agent }: { agent: Agent }) => createElement('span', {}, agent.name),
}))
vi.mock('./FindingsTab', () => ({ FindingsTab: () => null }))
vi.mock('./FleetTab', () => ({ FleetTab: () => null }))
vi.mock('./RunsTab', () => ({ RunsTab: () => null }))
vi.mock('./SelfHealingTab', () => ({ SelfHealingTab: () => null }))
vi.mock('./LensList', () => ({
  LensList: ({ onScanLens }: { onScanLens: (id: string) => void }) =>
    createElement('button', { onClick: () => onScanLens('LENS1') }, 'scan-one-lens'),
}))

beforeEach(() => {
  vi.clearAllMocks()
  calls.getInsightSettings.mockResolvedValue({ maxSessions: 25 })
  calls.listAgents.mockResolvedValue([
    { id: 'OLD', name: 'Archived', archived: true },
    { id: 'AGT1', name: 'Ada' },
    { id: 'AGT2', name: 'Bryn' },
  ])
  calls.listInsightLenses.mockResolvedValue([])
  calls.listInsightFindings.mockResolvedValue([])
  calls.getInsightScanStatus.mockResolvedValue({ scanning: false })
  calls.updateInsightSettings.mockImplementation(async (settings: InsightSettings) => settings)
  calls.runInsightScan.mockResolvedValue({})
})

async function renderPanel(tab = 'findings') {
  const onError = vi.fn()
  const rendered = render(createElement(InsightPanel, { tab, onError }))
  await act(async () => {})
  return { ...rendered, onError }
}

const agents = [{ id: 'AGT1' }, { id: 'AGT2' }] as Agent[]

describe('InsightPanel analysis agent selection', () => {
  it('defaults an empty selection to the first loaded agent', () => {
    expect(withDefaultAnalysisAgent({}, agents)).toEqual({ autoScanAgentId: 'AGT1' })
    expect(withDefaultAnalysisAgent({}, [])).toEqual({})
    expect(withDefaultAnalysisAgent({ autoScanAgentId: 'AGT2' }, agents)).toEqual({
      autoScanAgentId: 'AGT2',
    })
  })

  it('never defaults to an archived agent', () => {
    const withArchived = [{ id: 'AGT0', archived: true }, ...agents] as Agent[]
    expect(withDefaultAnalysisAgent({}, withArchived)).toEqual({ autoScanAgentId: 'AGT1' })
    expect(withDefaultAnalysisAgent({}, [withArchived[0]])).toEqual({})
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

  it('loads the live default into the rail picker and persists a changed selection', async () => {
    const { container, onError } = await renderPanel('settings')
    const rail = query<HTMLElement>(container, '[data-testid="scan-rail"]')
    const trigger = query<HTMLButtonElement>(rail, '[data-testid="agent-picker-trigger"]')
    const scan = query<HTMLButtonElement>(rail, 'button[title="scan.startTitle"]')
    expect(trigger.textContent).toContain('Ada')
    expect(scan.compareDocumentPosition(trigger) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(container.querySelectorAll('[data-testid="agent-picker-trigger"]')).toHaveLength(1)

    act(() => trigger.click())
    const options = [
      ...rail.querySelectorAll<HTMLButtonElement>('[data-testid="agent-picker-option"]'),
    ]
    expect(options.map((option) => option.textContent)).toEqual(['Ada', 'Bryn'])
    await act(async () => options[1].click())
    expect(calls.updateInsightSettings).toHaveBeenCalledWith({
      maxSessions: 25,
      autoScanAgentId: 'AGT2',
    })
    expect(trigger.textContent).toContain('Bryn')
    expect(onError).not.toHaveBeenCalled()
  })

  it('starts a manual scan without overriding the persisted backend agent', async () => {
    calls.getInsightSettings.mockResolvedValue({ autoScanAgentId: 'AGT2' })
    const { container } = await renderPanel()
    const scan = query<HTMLButtonElement>(container, 'button[title="scan.startTitle"]')
    await act(async () => scan.click())
    expect(calls.runInsightScan).toHaveBeenCalledWith({})
    expect(scan.disabled).toBe(true)
  })

  it('forwards only the selected lens when starting a lens scan', async () => {
    const { container } = await renderPanel('lenses')
    const scan = [...container.querySelectorAll<HTMLButtonElement>('button')].find(
      (button) => button.textContent === 'scan-one-lens',
    )!
    await act(async () => scan.click())
    expect(calls.runInsightScan).toHaveBeenCalledWith({ lensIds: ['LENS1'] })
  })
})
