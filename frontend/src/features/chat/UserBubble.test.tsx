// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it } from 'vitest'
import { UserBubble } from './UserBubble'

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: ReturnType<typeof createRoot>[] = []

function renderBubble(text: string) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  act(() => root.render(<UserBubble text={text} agents={[]} />))
  return container
}

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

// The command bubble leads with a ⌘ glyph; the prose bubble never does (its
// Markdown may still use monospace for path chips, so don't key on font-mono).
const isCommandStyle = (c: HTMLElement) => (c.textContent ?? '').startsWith('⌘')

describe('UserBubble command style', () => {
  it('renders a slash command in the command style', () => {
    expect(isCommandStyle(renderBubble('/compact now'))).toBe(true)
  })

  it('renders a message starting with an absolute POSIX path as prose', () => {
    expect(isCommandStyle(renderBubble('/Users/me/app/main.go fails'))).toBe(false)
  })

  it('unwraps a quoted command but not a quoted path', () => {
    const quotedCmd = renderBubble('"/compact"')
    expect(isCommandStyle(quotedCmd)).toBe(false)
    expect(quotedCmd.textContent).toContain('/compact')
    expect(quotedCmd.textContent).not.toContain('"')

    const quotedPath = renderBubble('"/tmp/x"')
    expect(quotedPath.textContent).toContain('"/tmp/x"')
  })
})
