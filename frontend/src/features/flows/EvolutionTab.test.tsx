// @vitest-environment jsdom
import { act } from 'react'
import { beforeEach, expect, it, vi } from 'vitest'
import type { Flow, FlowProposal } from '@/types'
import { i18next } from '@/i18n'
import { query, render } from '@/test/render'
import { EvolutionTab } from './EvolutionTab'

const ws = vi.hoisted(() => ({
  get: vi.fn(),
  update: vi.fn(),
}))
vi.mock('@/api/workspaces', () => ({
  workspaceApi: { getWorkspaceSettings: ws.get, updateWorkspaceSettings: ws.update },
}))
vi.mock('@/api', () => ({ api: {} }))
vi.mock('@/shared/components/agents/SystemRolePickers', () => ({
  SystemRoleAssignments: () => null,
}))

const flow = {
  id: 'FLW1',
  agentId: 'AGT1',
  version: 2,
  policy: {
    mode: 'propose',
    everyRuns: 5,
    minConfidence: 0.7,
    maxNodes: 16,
    allowPromptChanges: true,
  },
  stats: { runs: 3, runsAtOptimize: 3 },
} as unknown as Flow

const applied: FlowProposal = {
  id: 'FPR1',
  flowId: 'FLW1',
  agentId: 'AGT1',
  baseVersion: 1,
  author: { kind: 'observer', id: 'AGT9' },
  reason: 'add a critic',
  confidence: 0.9,
  status: 'applied',
  appliedVersion: 2,
  autoApplied: 'workspace',
  createdAt: 1,
}

async function flush() {
  await act(async () => {
    await Promise.resolve()
  })
}

beforeEach(async () => {
  await i18next.changeLanguage('en')
  ws.get.mockReset().mockResolvedValue({ flowAutoAccept: false })
  ws.update.mockReset().mockResolvedValue({ flowAutoAccept: true })
})

it('saves the workspace auto-accept switch at once and refreshes the flow', async () => {
  const onChanged = vi.fn()
  const { container } = render(
    <EvolutionTab
      flow={flow}
      versions={[]}
      proposals={[]}
      promptVersions={[]}
      busy={false}
      onChanged={onChanged}
      onError={vi.fn()}
    />,
  )
  await flush()
  const box = query<HTMLInputElement>(container, '[data-testid="flow-ws-auto-accept"]')
  expect(box.checked).toBe(false)
  expect(container.querySelector('[data-testid="flow-ws-auto-accept-active"]')).toBeNull()
  act(() => box.click())
  await flush()
  expect(ws.update).toHaveBeenCalledWith({ flowAutoAccept: true })
  expect(box.checked).toBe(true)
  expect(onChanged).toHaveBeenCalledTimes(1)
  expect(query(container, '[data-testid="flow-ws-auto-accept-active"]').textContent).toContain(
    'behaves as Auto',
  )
})

it('reverts the switch when the save fails', async () => {
  ws.update.mockRejectedValue(new Error('nope'))
  const onError = vi.fn()
  const { container } = render(
    <EvolutionTab
      flow={flow}
      versions={[]}
      proposals={[]}
      promptVersions={[]}
      busy={false}
      onChanged={vi.fn()}
      onError={onError}
    />,
  )
  await flush()
  const box = query<HTMLInputElement>(container, '[data-testid="flow-ws-auto-accept"]')
  act(() => box.click())
  await flush()
  expect(onError).toHaveBeenCalledWith('nope')
  expect(box.checked).toBe(false)
})

it('marks an auto-accepted proposal with the setting that applied it', async () => {
  const { container } = render(
    <EvolutionTab
      flow={flow}
      versions={[]}
      proposals={[applied, { ...applied, id: 'FPR2', autoApplied: undefined }]}
      promptVersions={[]}
      busy={false}
      onChanged={vi.fn()}
      onError={vi.fn()}
    />,
  )
  await flush()
  const badges = container.querySelectorAll('[data-testid="flow-proposal-auto"]')
  expect(badges).toHaveLength(1)
  expect(badges[0].textContent).toBe('auto-accepted · workspace')
})
