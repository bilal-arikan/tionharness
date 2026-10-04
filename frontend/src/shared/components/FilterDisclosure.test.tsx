// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { FilterToggle } from './FilterDisclosure'
import { useFilterDisclosure } from './useFilterDisclosure'
import { setLocale } from '@/i18n'

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: ReturnType<typeof createRoot>[] = []

beforeEach(async () => {
  await setLocale('tr')
  localStorage.clear()
})

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

function render(node: React.ReactNode) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  act(() => root.render(node))
  return container
}

function Strip({ active = 0 }: { active?: number }) {
  const [open, toggle] = useFilterDisclosure('test')
  return (
    <div>
      <FilterToggle open={open} onToggle={toggle} activeCount={active} testId="toggle" />
      {open && <span data-testid="chips">chips</span>}
    </div>
  )
}

describe('FilterToggle + useFilterDisclosure', () => {
  it('folds the strip and remembers the choice', () => {
    const view = render(<Strip />)
    const button = view.querySelector<HTMLButtonElement>('[data-testid="toggle"]')!
    expect(button.getAttribute('aria-expanded')).toBe('true')
    expect(view.querySelector('[data-testid="chips"]')).not.toBeNull()

    act(() => button.click())
    expect(button.getAttribute('aria-expanded')).toBe('false')
    expect(view.querySelector('[data-testid="chips"]')).toBeNull()
    expect(localStorage.getItem('tionharness.filtersOpen.test')).toBe('0')

    // A remount reads the stored state back.
    const again = render(<Strip />)
    expect(again.querySelector('[data-testid="chips"]')).toBeNull()
  })

  it('badges active filters so a folded strip still says it is narrowing', () => {
    const view = render(<Strip active={3} />)
    const button = view.querySelector<HTMLButtonElement>('[data-testid="toggle"]')!
    expect(button.textContent).toContain('Filtreler')
    expect(button.textContent).toContain('3')
  })
})
