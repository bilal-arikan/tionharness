// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it } from 'vitest'
import { ActivityCard } from './ActivityCard'

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: ReturnType<typeof createRoot>[] = []

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

describe('ActivityCard collab metadata', () => {
  it('shows structured lifecycle data without rendering prompt-like input or output', () => {
    const container = document.createElement('div')
    document.body.appendChild(container)
    const root = createRoot(container)
    roots.push(root)
    act(() =>
      root.render(
        <ActivityCard
          step={{
            kind: 'tool',
            tool: 'collab_tool_call',
            operation: 'spawn_agent',
            target: ['thread-a', 'thread-b'],
            status: 'completed',
            durationMs: 1250,
            summary: 'spawn_agent · receivers: thread-a, thread-b · completed',
            input: { prompt: 'TOP SECRET PROMPT' },
            output: 'TOP SECRET PROMPT',
          }}
        />,
      ),
    )

    const button = container.querySelector('button')
    if (!button) throw new Error('Header button not found')
    expect(button.textContent).toContain('spawn_agent')
    expect(button.textContent).toContain('completed')
    expect(button.textContent).toContain('1.3 sn')
    expect(container.textContent).not.toContain('TOP SECRET PROMPT')

    act(() => button.click())
    expect(container.textContent).toContain('Hedef: thread-a, thread-b')
    expect(container.textContent).toContain('Durum: completed')
    expect(container.textContent).not.toContain('TOP SECRET PROMPT')
  })
})
