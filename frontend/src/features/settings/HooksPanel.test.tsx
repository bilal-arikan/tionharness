// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Hook } from '@/types'
import { i18next } from '@/i18n'

const hooks: Hook[] = [
  {
    id: 'H1',
    event: 'PreToolUse',
    matcher: 'Bash',
    type: 'command',
    command: '$j=[Console]::In.ReadToEnd()|ConvertFrom-Json',
    shellWarning: 'powershell-missing',
    timeoutSec: 30,
    enabled: true,
  },
  {
    id: 'H2',
    event: 'PostToolUse',
    matcher: '',
    type: 'command',
    command: 'sqz hook claude',
    timeoutSec: 30,
    enabled: true,
  },
]

vi.mock('@/api', () => ({
  api: {
    listHooks: vi.fn(() => Promise.resolve(hooks)),
    listBuiltinHooks: vi.fn(() => Promise.resolve([])),
  },
}))

const { HooksPanel } = await import('./HooksPanel')

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: ReturnType<typeof createRoot>[] = []

beforeEach(async () => {
  await i18next.changeLanguage('en')
})

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

describe('HooksPanel shell warning', () => {
  it('flags only the hook the server reports as unrunnable on its OS', async () => {
    const container = document.createElement('div')
    document.body.appendChild(container)
    const root = createRoot(container)
    roots.push(root)
    await act(async () => {
      root.render(<HooksPanel onError={() => {}} />)
    })
    const warnings = container.querySelectorAll('[data-testid="hook-shell-warning"]')
    expect(warnings).toHaveLength(1)
    expect(warnings[0].textContent).toContain('pwsh')
  })
})
