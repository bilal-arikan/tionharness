// @vitest-environment jsdom

import { act, createElement, type ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { InsightSettings } from '@/types'
import { render, query } from '@/test/render'
import { InsightPanel } from './InsightPanel'

const calls = vi.hoisted(() => ({
  getInsightSettings: vi.fn(),
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
vi.mock('@/shared/components/agents/SystemRolePickers', () => ({
  SystemRoleSelect: ({ role }: { role: string }) =>
    createElement('div', { 'data-testid': 'role-select', 'data-role': role }),
  SystemRoleAssignments: () => null,
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

describe('InsightPanel analysis agent', () => {
  it('offers the Insight analyzer role picker under the scan button', async () => {
    const { container } = await renderPanel('settings')
    const rail = query<HTMLElement>(container, '[data-testid="scan-rail"]')
    const picker = query<HTMLElement>(rail, '[data-testid="role-select"]')
    const scan = query<HTMLButtonElement>(rail, 'button[title="scan.startTitle"]')
    expect(picker.dataset.role).toBe('insight')
    expect(scan.compareDocumentPosition(picker) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(calls.updateInsightSettings).not.toHaveBeenCalled()
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
