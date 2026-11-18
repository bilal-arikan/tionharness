// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it } from 'vitest'
import type { Skill } from '@/types'
import { SkillRowBody } from './SkillRowBody'

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: ReturnType<typeof createRoot>[] = []

const skill = {
  slug: 'daily-weather-report',
  name: 'Günlük Hava Durumu Raporu',
  description: 'Daily forecast for the configured city.',
  source: 'workspace',
  shared: false,
  visibility: 'name-only',
} as unknown as Skill

function renderRow(sk: Skill) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  act(() => root.render(<SkillRowBody skill={sk} />))
  return container
}

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.innerHTML = ''
})

describe('SkillRowBody', () => {
  it('gives the name its own line, with no badge beside it', () => {
    const container = renderRow(skill)
    const name = container.querySelector<HTMLElement>('[data-testid="skill-row-name"]')!
    expect(name.textContent).toBe('Günlük Hava Durumu Raporu')
    expect(name.title).toBe('Günlük Hava Durumu Raporu')
    // The name is a block of its own: nothing else shares its line.
    expect(name.className).toContain('block')
    expect(name.children).toHaveLength(0)
  })

  it('puts the access, tier and source badges on the meta line', () => {
    const container = renderRow(skill)
    const meta = container.querySelector<HTMLElement>('[data-testid="skill-row-meta"]')!
    // restricted + visibility tier + source
    expect(meta.children.length).toBeGreaterThanOrEqual(3)
    // One quiet style: no coloured inline fill, no uppercase chip.
    for (const badge of Array.from(meta.children) as HTMLElement[]) {
      expect(badge.style.backgroundColor).toBe('')
      expect(badge.className).not.toContain('uppercase')
    }
  })

  it('omits the restricted badge for a shared skill', () => {
    const shared = renderRow({ ...skill, shared: true } as Skill)
    const restricted = renderRow(skill)
    const count = (c: HTMLElement) =>
      c.querySelector('[data-testid="skill-row-meta"]')!.children.length
    expect(count(restricted) - count(shared)).toBe(1)
  })
})
