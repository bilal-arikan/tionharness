// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AppSettings } from '@/types'
import { SelfHealingTab } from './SelfHealingTab'

const apiMock = vi.hoisted(() => ({
  getSettings: vi.fn(),
  updateSettings: vi.fn(),
}))

vi.mock('@/api', () => ({ api: apiMock }))

// Only the fields SelfHealingTab reads matter; the rest of AppSettings is
// irrelevant to the Save gate under test.
const settings = {
  toolGuardWarnings: true,
  toolGuardHardStop: false,
  guardExactWarn: 2,
  guardExactBlock: 5,
  guardSameToolWarn: 3,
  guardSameToolHalt: 8,
  guardNoProgressWarn: 2,
  guardNoProgressBlock: 5,
  stuckTurnThreshold: 3,
  lessonReflect: true,
} as unknown as AppSettings

describe('SelfHealingTab save gate', () => {
  let container: HTMLDivElement
  let root: Root | null

  beforeEach(() => {
    vi.clearAllMocks()
    apiMock.getSettings.mockResolvedValue(settings)
    container = document.createElement('div')
    document.body.appendChild(container)
    root = null
    ;(
      globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true
  })

  afterEach(() => {
    act(() => root?.unmount())
    container.remove()
  })

  const saveButton = () =>
    container.querySelector<HTMLButtonElement>('[data-testid="self-healing-save"]')!

  const numberInput = (index: number) =>
    container.querySelectorAll<HTMLInputElement>('input[type="number"]')[index]

  const type = (input: HTMLInputElement, value: string) => {
    act(() => {
      const setter = Object.getOwnPropertyDescriptor(
        window.HTMLInputElement.prototype,
        'value',
      )?.set
      setter?.call(input, value)
      input.dispatchEvent(new Event('input', { bubbles: true }))
    })
  }

  it('keeps Save disabled while a number field holds an invalid value', async () => {
    await act(async () => {
      root = createRoot(container)
      root.render(<SelfHealingTab onError={() => {}} />)
    })

    expect(saveButton().disabled).toBe(true) // nothing edited yet

    // Reject an out-of-range value: it must not reach the draft, and it must
    // surface inline.
    type(numberInput(0), '999')
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('50')
    expect(saveButton().disabled).toBe(true)

    // Editing a *different* field makes the draft dirty — this is exactly the
    // case that used to unlock Save and write the stale value back.
    type(numberInput(1), '6')
    expect(saveButton().disabled).toBe(true)

    // Fixing the invalid field releases the gate.
    type(numberInput(0), '4')
    expect(container.querySelector('[role="alert"]')).toBeNull()
    expect(saveButton().disabled).toBe(false)
  })

  it('writes only the self-healing fields and never a lesson lifetime', async () => {
    apiMock.updateSettings.mockImplementation(async (patch: Partial<AppSettings>) => ({
      ...settings,
      ...patch,
    }))
    await act(async () => {
      root = createRoot(container)
      root.render(<SelfHealingTab onError={() => {}} />)
    })
    type(numberInput(0), '4')
    await act(async () => saveButton().click())
    expect(apiMock.updateSettings).toHaveBeenCalledTimes(1)
    const patch = apiMock.updateSettings.mock.calls[0][0] as Record<string, unknown>
    expect(patch.guardExactWarn).toBe(4)
    expect(patch.lessonReflect).toBe(true)
    expect(patch).not.toHaveProperty('lessonMaxAgeDays')
  })
})
