// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { MCPServer } from '@/types'
import { ServerRow } from './ServerRow'

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: ReturnType<typeof createRoot>[] = []

const server = {
  id: 'MCP1',
  name: 'codebase-memory-mcp',
  transport: 'stdio',
  command: 'codebase-memory-mcp',
  args: '["--stdio"]',
  url: '',
  enabled: true,
  scope: 'shared',
} as unknown as MCPServer

function renderRow(overrides: Partial<Parameters<typeof ServerRow>[0]> = {}) {
  const handlers = {
    onTest: vi.fn(),
    onCopy: vi.fn(),
    onToggle: vi.fn(),
    onEdit: vi.fn(),
    onRemove: vi.fn(),
    onVisibility: vi.fn(),
  }
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  act(() =>
    root.render(
      <ServerRow
        server={server}
        toolCount={3}
        poolStats={null}
        testing={false}
        editing={false}
        {...handlers}
        {...overrides}
      />,
    ),
  )
  return { container, handlers }
}

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.innerHTML = ''
})

describe('ServerRow', () => {
  it('keeps the whole name in one truncating element with a tooltip', () => {
    const { container } = renderRow()
    const name = container.querySelector<HTMLElement>('[data-testid="mcp-server-name"]')!
    expect(name.textContent).toBe('codebase-memory-mcp')
    expect(name.title).toBe('codebase-memory-mcp')
    expect(name.className).toContain('truncate')
  })

  it('stacks identity and actions below sm and lets the actions wrap', () => {
    const { container } = renderRow()
    const layout = container.querySelector('[data-testid="mcp-server-row"]')!.firstElementChild!
    expect(layout.className).toContain('flex-col')
    expect(layout.className).toContain('sm:flex-row')
    const actions = container.querySelector('[data-testid="mcp-server-test"]')!.parentElement!
    expect(actions.className).toContain('flex-wrap')
  })

  it('wires every action to its handler', () => {
    const { container, handlers } = renderRow()
    const click = (testId: string) =>
      act(() => container.querySelector<HTMLButtonElement>(`[data-testid="${testId}"]`)!.click())
    click('mcp-server-test')
    click('mcp-server-copy')
    click('mcp-server-toggle')
    click('mcp-server-edit')
    click('mcp-server-delete')
    click('mcp-server-visibility-all')
    expect(handlers.onTest).toHaveBeenCalledTimes(1)
    expect(handlers.onCopy).toHaveBeenCalledTimes(1)
    expect(handlers.onToggle).toHaveBeenCalledTimes(1)
    expect(handlers.onEdit).toHaveBeenCalledTimes(1)
    expect(handlers.onRemove).toHaveBeenCalledTimes(1)
    expect(handlers.onVisibility).toHaveBeenCalledTimes(1)
  })

  it('shows the test result and hides the bulk-visibility row without tools', () => {
    const { container } = renderRow({ toolCount: 0, testResult: '12 tools' })
    expect(container.textContent).toContain('12 tools')
    expect(container.querySelector('[data-testid="mcp-server-visibility-all"]')).toBeNull()
  })
})
