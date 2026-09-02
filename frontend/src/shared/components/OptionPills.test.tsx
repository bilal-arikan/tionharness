// @vitest-environment jsdom

import { act, useState } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, describe, expect, it } from 'vitest'
import { OptionPills, type PillOption } from './OptionPills'

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: Root[] = []
const options: PillOption[] = [
  { value: 'one', label: 'Bir', icon: '●' },
  { value: 'two', label: 'İki', disabled: true },
  { value: 'three', label: 'Üç', icon: '○' },
]

function renderPills() {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)

  function Harness() {
    const [value, setValue] = useState('one')
    return (
      <OptionPills
        value={value}
        onChange={setValue}
        options={options}
        ariaLabel="Sınama seçenekleri"
        ariaDescribedBy="pill-description"
        testid="test-pills"
      />
    )
  }

  act(() => root.render(<Harness />))
  return container
}

function pill(container: HTMLElement, value: string) {
  const element = container.querySelector<HTMLButtonElement>(
    `[data-testid="test-pills-option"][data-value="${value}"]`,
  )
  if (!element) throw new Error(`Pill not found: ${value}`)
  return element
}

function press(element: HTMLElement, key: string) {
  act(() => element.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true })))
}

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

describe('OptionPills', () => {
  it('uses one tab stop and skips disabled options during arrow navigation', () => {
    const container = renderPills()
    const one = pill(container, 'one')
    const two = pill(container, 'two')
    const three = pill(container, 'three')

    expect(one.tabIndex).toBe(0)
    expect(two.tabIndex).toBe(-1)
    expect(three.tabIndex).toBe(-1)

    act(() => one.focus())
    press(one, 'ArrowRight')

    expect(document.activeElement).toBe(three)
    expect(three.getAttribute('aria-checked')).toBe('true')
    expect(three.tabIndex).toBe(0)
    expect(one.tabIndex).toBe(-1)
  })

  it('supports vertical arrows, Home, End and wrapped reverse navigation', () => {
    const container = renderPills()
    const one = pill(container, 'one')
    const three = pill(container, 'three')

    press(one, 'End')
    expect(document.activeElement).toBe(three)

    press(three, 'Home')
    expect(document.activeElement).toBe(one)

    press(one, 'ArrowLeft')
    expect(document.activeElement).toBe(three)
    expect(three.getAttribute('aria-checked')).toBe('true')

    press(three, 'ArrowDown')
    expect(document.activeElement).toBe(one)

    press(one, 'ArrowUp')
    expect(document.activeElement).toBe(three)
  })

  it('connects the description and hides decorative icons from the accessible name', () => {
    const container = renderPills()
    const group = container.querySelector('[data-testid="test-pills"]')
    const icon = pill(container, 'one').querySelector('span')

    expect(group?.getAttribute('aria-describedby')).toBe('pill-description')
    expect(icon?.getAttribute('aria-hidden')).toBe('true')
  })
})
