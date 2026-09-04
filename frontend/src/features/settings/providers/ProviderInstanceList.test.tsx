// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ProviderInstance } from '@/api/providers'
import { ProviderInstanceList } from './ProviderInstanceList'

let root: Root | undefined
let host: HTMLDivElement | undefined

afterEach(() => {
  if (root) act(() => root!.unmount())
  host?.remove()
  root = undefined
  host = undefined
})

function instance(over: Partial<ProviderInstance> = {}): ProviderInstance {
  return {
    id: 'p1',
    kindId: 'lmstudio',
    label: 'Yerel',
    icon: '',
    enabled: true,
    defaultModel: 'qwen3-8b',
    models: '',
    config: {},
    secretsSet: {},
    createdAt: '2026-09-04T00:00:00Z',
    ...over,
  }
}

function renderList(instances: ProviderInstance[]): HTMLElement {
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
  act(() => {
    root!.render(
      <ProviderInstanceList
        instances={instances}
        kinds={[]}
        onEdit={vi.fn()}
        onDelete={vi.fn()}
        authById={{}}
        onAuthOpen={vi.fn()}
      />,
    )
  })
  return host
}

// The offline badge exists so a configured-but-closed local server is visible
// before an agent turn wastes itself on a refused connection. The three cases
// below are what the tri-state `reachable` field is for.
describe('ProviderInstanceList local reachability', () => {
  it('flags a local server that is not answering', () => {
    expect(renderList([instance({ reachable: false })]).textContent).toContain('sunucu kapalı')
  })

  it('shows no badge while the local server answers', () => {
    expect(renderList([instance({ reachable: true })]).textContent).not.toContain('sunucu kapalı')
  })

  // Absent means "not probed" (every hosted kind), NOT "down" — flagging those
  // would mark every working Anthropic/OpenRouter instance as offline.
  it('shows no badge for a hosted provider, which is never probed', () => {
    const html = renderList([instance({ kindId: 'anthropic', reachable: undefined })])
    expect(html.textContent).not.toContain('sunucu kapalı')
  })
})
