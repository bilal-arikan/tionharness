// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ArchiveViewBanner, ArchiveViewToggle } from './ArchiveView'

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: ReturnType<typeof createRoot>[] = []

function render(node: React.ReactNode) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  act(() => root.render(node))
  return container
}

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

describe('ArchiveViewToggle', () => {
  it('labels the way into the archive and back, and reports its state', () => {
    const onToggle = vi.fn()
    const off = render(<ArchiveViewToggle testId="t" active={false} onToggle={onToggle} />)
    const button = off.querySelector<HTMLButtonElement>('[data-testid="t"]')!
    expect(button.textContent).toContain('Arşiv')
    expect(button.getAttribute('aria-pressed')).toBe('false')
    act(() => button.click())
    expect(onToggle).toHaveBeenCalledOnce()

    const on = render(
      <ArchiveViewToggle testId="t2" active onToggle={onToggle} backLabel="Panoya dön" />,
    )
    const back = on.querySelector<HTMLButtonElement>('[data-testid="t2"]')!
    expect(back.textContent).toContain('Panoya dön')
    expect(back.getAttribute('aria-pressed')).toBe('true')
  })
})

describe('ArchiveViewBanner', () => {
  it('says the archive is empty or counts its items with the restore hint', () => {
    expect(render(<ArchiveViewBanner count={0} noun="ajan" />).textContent).toContain(
      'Arşivlenmiş ajan yok.',
    )
    expect(
      render(<ArchiveViewBanner count={2} noun="skill" restoreHint="geri al" />).textContent,
    ).toContain('2 arşivlenmiş skill — geri al')
  })
})
