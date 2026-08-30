// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it } from 'vitest'
import type { TurnStep } from '@/types'
import { CompactionCard } from './CompactionCard'

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: ReturnType<typeof createRoot>[] = []

function renderCard(step: TurnStep) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  act(() => root.render(<CompactionCard step={step} />))
  return container
}

function header(container: HTMLElement) {
  const button = container.querySelector('button')
  if (!button) throw new Error('Header button not found')
  return button
}

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

describe('CompactionCard', () => {
  it('renders an in-progress native compaction without invented token figures', () => {
    const container = renderCard({
      kind: 'compaction',
      id: 'cmp-1',
      running: true,
      source: 'cli-native',
      provider: 'codex-cli',
      sessionAction: 'native-compact',
    })

    const text = header(container).textContent ?? ''
    expect(text).toContain('CLI bağlamı sıkıştırılıyor…')
    expect(text).not.toContain('tok')
  })

  it('renders the folded message count and the token badge from structural fields', () => {
    const container = renderCard({
      kind: 'compaction',
      text: '🗜 Bağlam otomatik sıkıştırıldı — 12 mesaj kalıcı özete katlandı.',
      foldedMsgs: 12,
      beforeTokens: 48000,
      afterTokens: 9000,
      trigger: 'auto',
    })

    const text = header(container).textContent ?? ''
    expect(text).toContain('12 mesaj özete katlandı')
    expect(text).toContain('48.0k')
    expect(text).toContain('9.0k')
    expect(text).toContain('tok')
  })

  it('falls back to the text headline when the structural fields are absent', () => {
    const container = renderCard({
      kind: 'compaction',
      text: '🗜 Bağlam otomatik sıkıştırıldı — 4 mesaj kalıcı özete katlandı.',
    })

    const text = header(container).textContent ?? ''
    expect(text).toContain('🗜 Bağlam otomatik sıkıştırıldı — 4 mesaj kalıcı özete katlandı.')
    // No before/after figures → no token badge.
    expect(text).not.toContain('tok')
  })

  it('renders a Turkish label for a known trigger', () => {
    const container = renderCard({
      kind: 'compaction',
      foldedMsgs: 5,
      trigger: 'reactive',
    })

    act(() => header(container).click())
    expect(container.textContent).toContain('Tetikleyici:')
    expect(container.textContent).toContain('taşma kurtarması')
  })

  it('renders an unknown trigger verbatim instead of hiding it', () => {
    const container = renderCard({
      kind: 'compaction',
      foldedMsgs: 5,
      trigger: 'weird',
    })

    act(() => header(container).click())
    expect(container.textContent).toContain('weird')
  })

  it.each([
    [{ source: 'tionharness' }, 'Kaynak: TionHarness'],
    [{ source: 'cli-native' }, 'Kaynak: CLI yerel'],
    [{ provider: 'claude-cli' }, 'Sağlayıcı: Claude CLI'],
    [{ provider: 'codex-cli' }, 'Sağlayıcı: Codex CLI'],
    [{ sessionAction: 'resume' }, 'Oturum işlemi: oturumu sürdür'],
    [{ sessionAction: 'native-compact' }, 'Oturum işlemi: yerel sıkıştırma'],
    [{ sessionAction: 'restart-summary' }, 'Oturum işlemi: özetle yeniden başlat'],
  ] as const)('renders the Turkish compaction metadata label %#', (payload, expected) => {
    const container = renderCard({ kind: 'compaction', ...payload })

    act(() => header(container).click())
    expect(container.textContent).toContain(expected)
  })

  it('renders unknown provenance and session action values verbatim', () => {
    const container = renderCard({
      kind: 'compaction',
      source: 'future-source',
      provider: 'future-provider',
      sessionAction: 'future-action',
    })

    act(() => header(container).click())
    expect(container.textContent).toContain('future-source')
    expect(container.textContent).toContain('future-provider')
    expect(container.textContent).toContain('future-action')
  })
})
