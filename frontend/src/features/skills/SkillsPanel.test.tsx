// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { SkillDetail } from '@/types'
import { clearSessionState } from '@/shared/hooks/useSessionState'
import { SkillsPanel } from './SkillsPanel'

const apiMock = vi.hoisted(() => ({
  listSkills: vi.fn(),
  getSkill: vi.fn(),
  restoreSkill: vi.fn(),
}))

vi.mock('@/api', () => ({ api: apiMock }))

const roots: ReturnType<typeof createRoot>[] = []
const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const editedSkill: SkillDetail = {
  slug: 'edited-skill',
  name: 'Edited Skill',
  description: 'A shipped skill changed by the user',
  source: 'global',
  shared: true,
  visibility: 'full',
  defaultState: 'edited',
  body: '# Changed body',
  dir: 'C:/skills/edited-skill',
}

function renderPanel(onError = vi.fn()) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  act(() => root.render(<SkillsPanel onError={onError} />))
  return { container, onError }
}

async function flush() {
  await act(async () => {
    await Promise.resolve()
    await Promise.resolve()
  })
}

function button(container: HTMLElement, text: string) {
  const match = [...container.querySelectorAll('button')].find((item) =>
    item.textContent?.includes(text),
  )
  if (!match) throw new Error(`Button not found: ${text}`)
  return match
}

beforeEach(() => {
  clearSessionState('skills.activeSlug')
  vi.clearAllMocks()
  apiMock.listSkills.mockResolvedValue([editedSkill])
  apiMock.getSkill.mockResolvedValue(editedSkill)
  apiMock.restoreSkill.mockResolvedValue({ ...editedSkill, defaultState: 'default' })
})

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

describe('SkillsPanel shipped skill state', () => {
  it('shows the edited badge in the list and selected-skill header', async () => {
    const { container } = renderPanel()
    await flush()

    expect(container.textContent?.match(/düzenlendi/g)).toHaveLength(2)
    expect(button(container, 'Varsayılan')).toBeTruthy()
  })

  it('restores the default after confirmation and refreshes edited UI state', async () => {
    const restored = { ...editedSkill, defaultState: 'default' as const, body: '# Default body' }
    apiMock.listSkills.mockResolvedValueOnce([editedSkill]).mockResolvedValue([restored])
    apiMock.getSkill.mockResolvedValueOnce(editedSkill).mockResolvedValue(restored)
    const { container, onError } = renderPanel()
    await flush()

    act(() => button(container, 'Varsayılan').click())
    expect(container.textContent).toContain('Düzenlemeler silinsin mi?')
    act(() => button(container, 'Evet').click())
    await flush()

    expect(apiMock.restoreSkill).toHaveBeenCalledOnce()
    expect(apiMock.restoreSkill).toHaveBeenCalledWith('edited-skill')
    expect(apiMock.listSkills).toHaveBeenCalledTimes(2)
    expect(apiMock.getSkill).toHaveBeenCalledTimes(2)
    expect(container.textContent).not.toContain('düzenlendi')
    expect(onError).not.toHaveBeenCalled()
  })

  it('keeps edited state and reports restore failures', async () => {
    apiMock.restoreSkill.mockRejectedValue(new Error('restore failed'))
    const { container, onError } = renderPanel()
    await flush()

    act(() => button(container, 'Varsayılan').click())
    act(() => button(container, 'Evet').click())
    await flush()

    expect(onError).toHaveBeenCalledWith('restore failed')
    expect(container.textContent).toContain('düzenlendi')
    expect(apiMock.listSkills).toHaveBeenCalledOnce()
  })
})
