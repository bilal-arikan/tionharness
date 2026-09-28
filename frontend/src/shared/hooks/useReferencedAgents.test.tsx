// @vitest-environment jsdom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { Agent } from '@/types'
import { useReferencedAgents } from './useReferencedAgents'

const getAgent = vi.hoisted(() => vi.fn())
vi.mock('@/api', () => ({ api: { getAgent } }))
let root: Root
let container: HTMLDivElement
let result: Agent[]
const live = { id: 'AGT1', name: 'Live' } as Agent
const archived = { id: 'AGT2', name: 'Archived author', archived: true } as Agent
const roster = [live]
function Harness({ ids, scope = 'WS5' }: { ids: string[]; scope?: string }) {
  result = useReferencedAgents(roster, ids, scope)
  return null
}
beforeEach(() => {
  vi.clearAllMocks()
  ;(
    globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
  ).IS_REACT_ACT_ENVIRONMENT = true
  getAgent.mockResolvedValue(archived)
  container = document.createElement('div')
  root = createRoot(container)
})
afterEach(() => act(() => root.unmount()))

it('fetches only missing referenced authors and releases them with the view', async () => {
  await act(async () => root.render(<Harness ids={['AGT1']} />))
  expect(getAgent).not.toHaveBeenCalled()
  await act(async () => root.render(<Harness ids={['AGT1', 'AGT2', 'AGT2', '*']} />))
  expect(getAgent).toHaveBeenCalledExactlyOnceWith('AGT2')
  expect(result).toEqual([live, archived])
  await act(async () => root.render(<Harness ids={['AGT1']} />))
  expect(result).toEqual([live])
})

it('does not retain a response from the previous workspace', async () => {
  let resolve!: (agent: Agent) => void
  getAgent.mockReturnValue(
    new Promise<Agent>((done) => {
      resolve = done
    }),
  )
  await act(async () => root.render(<Harness ids={['AGT2']} />))
  await act(async () => root.render(<Harness ids={[]} scope="WS10" />))
  await act(async () => resolve(archived))
  expect(result).toEqual([live])
})
