// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it } from 'vitest'
import { Markdown } from './Markdown'

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: ReturnType<typeof createRoot>[] = []

function renderMarkdown(source: string) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  act(() => root.render(<Markdown>{source}</Markdown>))
  return container
}

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

describe('Markdown links', () => {
  it('renders unsafe URL links as plain text', () => {
    const container = renderMarkdown('[unsafe](javascript:alert(1))')

    expect(container.textContent).toBe('unsafe')
    expect(container.querySelector('a')).toBeNull()
  })
})
