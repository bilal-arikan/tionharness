// @vitest-environment jsdom

import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import type { Pack } from '@/types'
import type { PriceTable } from '@/api/providers'
import { i18next } from '@/i18n'

vi.mock('@/shared/lib/catalog', () => ({
  useCatalog: () => [
    {
      id: 'claude-cli',
      label: 'Claude CLI',
      needsKey: false,
      allowCustomModel: true,
      appliesToolHooks: true,
      available: true,
      models: [
        { id: '', label: 'claude oturum modeli', resolvedModel: 'claude-opus-5' },
        { id: 'sonnet', label: 'Sonnet — dengeli' },
      ],
    },
    {
      id: 'codex-cli',
      label: 'Codex CLI',
      needsKey: false,
      allowCustomModel: true,
      appliesToolHooks: false,
      available: true,
      models: [{ id: '', label: 'codex oturum modeli' }],
    },
  ],
}))

import { PackPreview } from './PackPreview'

const prices = {} as PriceTable

function pack(kind: Pack['kind'], payload: NonNullable<Pack['payload']>): Pack {
  return { schema: 'v1', id: `test.${kind}`, kind, name: 'Test', description: '', payload }
}

describe('PackPreview model labels', () => {
  it('shows the resolved model for an agent pack with an empty CLI model', () => {
    const html = renderToStaticMarkup(
      <PackPreview
        pack={pack('agent', { agent: { name: 'Ada', provider: 'claude-cli', model: '' } })}
        prices={prices}
      />,
    )
    expect(html).toContain('Opus 5')
    expect(html).not.toContain('Varsayılan')
  })

  it('keeps a concrete agent model id', () => {
    const html = renderToStaticMarkup(
      <PackPreview
        pack={pack('agent', {
          agent: { name: 'Ada', provider: 'claude-cli', model: 'my-local-model' },
        })}
        prices={prices}
      />,
    )
    expect(html).toContain('my-local-model')
  })

  it('shows the neutral session label for an unresolved workspace agent', () => {
    const html = renderToStaticMarkup(
      <PackPreview
        pack={pack('workspace', {
          workspace: {
            name: 'Team',
            agents: [{ key: 'worker', name: 'Worker', provider: 'codex-cli', model: '' }],
          },
        })}
        prices={prices}
      />,
    )
    expect(html).toContain('codex oturum modeli')
    expect(html).not.toContain('Varsayılan')
  })

  it('renders preview chrome in the active UI language', async () => {
    const provider = pack('provider', {
      provider: {
        kind: 'openai',
        label: 'Test provider',
        baseUrl: 'https://example.invalid/v1',
        defaultModel: 'test-model',
        promptCache: 'auto',
      },
    })
    await i18next.changeLanguage('en')
    const english = renderToStaticMarkup(<PackPreview pack={provider} prices={prices} />)
    expect(english).toContain('Provider model')
    expect(english).toContain('Cache: ✅ automatic')

    await i18next.changeLanguage('tr')
    const turkish = renderToStaticMarkup(<PackPreview pack={provider} prices={prices} />)
    expect(turkish).toContain('Sağlayıcı modeli')
    expect(turkish).toContain('Cache: ✅ otomatik')
  })
})
