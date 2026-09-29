import { readdirSync, readFileSync } from 'node:fs'
import { afterEach, describe, expect, it } from 'vitest'
import { i18next } from '@/i18n'
import type { ProviderFieldSpec, ProviderKind } from '@/api/providers'
import type { CatalogEntry } from '@/types'
import { localizeCatalogEntry, localizeProviderKind } from './providerMetadata'

afterEach(async () => {
  await i18next.changeLanguage('tr')
})

function catalogEntry(patch: Partial<CatalogEntry> = {}): CatalogEntry {
  return {
    id: 'claude-cli',
    label: 'Claude CLI (abonelik · anahtarsız)',
    needsKey: false,
    allowCustomModel: true,
    available: true,
    appliesToolHooks: true,
    models: [
      {
        id: 'opus',
        label: 'Opus — en güçlü',
        description: 'En yetenekli; en yavaş/pahalı',
        thinkingTiers: ['low', 'high'],
      },
    ],
    ...patch,
  }
}

interface SourceModel {
  id: string
  label: string
  description: string
}

const turkishCopy =
  /[çğıöşüÇĞİÖŞÜ]|\b(?:oncu|onceki|hizli|ucuz|baglam|dusunme|yetenekli|guncel|amiral|genel|yerel|ucretsiz|guvenilir|yogun|gorev|modeli|katmani|dengeli|guclu|varyanti|topluluk|onerilen|sinif|hafif)\b/i

function readStaticProviderModels(): Map<string, SourceModel[]> {
  const providerDirectory = new URL('../../../../internal/providers/', import.meta.url)
  const providers = new Map<string, SourceModel[]>()

  for (const filename of readdirSync(providerDirectory)) {
    if (!/^kind_.*\.go$/.test(filename) || filename.endsWith('_test.go')) continue
    const source = readFileSync(new URL(filename, providerDirectory), 'utf8')
    const providerId = source.match(/Kind:\s+"([^"]+)"/)?.[1]
    if (!providerId) continue

    const models = [
      ...source.matchAll(/\{ID: "([^"]*)", Label: "([^"]*)", Description: "([^"]*)"\}/g),
    ].map(([, id, label, description]) => ({ id, label, description }))
    if (models.length > 0) providers.set(providerId, models)
  }

  const deepSeekModels = providers.get('deepseek')
  if (deepSeekModels) providers.set('deepseek-anthropic', deepSeekModels)
  return providers
}

describe('provider metadata localization', () => {
  it('switches known built-in catalog copy live without changing machine metadata', async () => {
    const localized = localizeCatalogEntry(catalogEntry())

    await i18next.changeLanguage('tr')
    expect(localized.label).toBe('Claude CLI (abonelik · anahtarsız)')
    expect(localized.models[0].label).toBe('Opus — en güçlü')
    expect(localized.models[0].description).toBe('En yetenekli; en yavaş ve en pahalı')

    await i18next.changeLanguage('en')
    expect(localized.label).toBe('Claude CLI (subscription · no API key)')
    expect(localized.models[0].label).toBe('Opus — most capable')
    expect(localized.models[0].description).toBe('Most capable; slowest and most expensive')
    expect(localized.id).toBe('claude-cli')
    expect(localized.models[0].id).toBe('opus')
    expect(localized.models[0].thinkingTiers).toEqual(['low', 'high'])
  })

  it('preserves custom labels and unknown remote model metadata verbatim', async () => {
    await i18next.changeLanguage('en')
    const localized = localizeCatalogEntry(
      catalogEntry({
        label: 'My Claude account',
        models: [{ id: 'remote-model-v9', label: 'Remote Model V9', description: 'Private' }],
      }),
    )

    expect(localized.label).toBe('My Claude account')
    expect(localized.models[0]).toMatchObject({
      id: 'remote-model-v9',
      label: 'Remote Model V9',
      description: 'Private',
    })
  })

  it('preserves every field of an unknown provider kind verbatim', async () => {
    await i18next.changeLanguage('en')
    const unknown: ProviderKind = {
      id: 'private-gateway',
      label: 'Özel Sağlayıcı',
      transport: 'api',
      multi: true,
      fields: [
        {
          key: 'key',
          label: 'Özel Erişim Bilgisi',
          type: 'password',
          required: true,
          secret: true,
          placeholder: 'özel değer',
          help: 'Kuruluşa özel açıklama',
        },
      ],
      models: [{ id: 'private-model', label: 'Özel Model', description: 'Özel açıklama' }],
    }

    expect(localizeProviderKind(unknown)).toBe(unknown)
    expect(localizeProviderKind(unknown)).toEqual(unknown)
  })

  it('supports the short built-in Anthropic-mode provider labels without matching custom labels', async () => {
    await i18next.changeLanguage('en')

    expect(
      localizeCatalogEntry(
        catalogEntry({ id: 'minimax-anthropic', label: 'MiniMax (Anthropic modu)', models: [] }),
      ).label,
    ).toBe('MiniMax (Anthropic mode · tool use + thinking)')
    expect(
      localizeCatalogEntry(
        catalogEntry({ id: 'deepseek-anthropic', label: 'DeepSeek (Anthropic modu)', models: [] }),
      ).label,
    ).toBe('DeepSeek (Anthropic mode · tool use + thinking)')
    expect(
      localizeCatalogEntry(
        catalogEntry({ id: 'minimax-anthropic', label: 'My Anthropic transport', models: [] }),
      ).label,
    ).toBe('My Anthropic transport')
  })

  it('localizes provider-kind fields while preserving field keys, types, and options', async () => {
    const kind: ProviderKind = {
      id: 'claude-cli',
      label: 'Claude CLI (abonelik · anahtarsız)',
      transport: 'cli',
      multi: true,
      fields: [
        {
          key: 'authKind',
          label: 'Kimlik Dogrulama Turu',
          type: 'select',
          options: ['', 'oauth', 'apikey'],
          required: false,
          help: "Bos = login'siz/varsayilan.",
          secret: false,
        },
      ],
    }
    const localized = localizeProviderKind(kind)

    await i18next.changeLanguage('en')
    expect(localized.fields[0].label).toBe('Authentication type')
    expect(localized.fields[0].help).toBe('Blank uses the default login without an override.')
    expect(localized.fields[0]).toMatchObject({
      key: 'authKind',
      type: 'select',
      options: ['', 'oauth', 'apikey'],
      required: false,
      secret: false,
    })
  })

  it('localizes representative field copy for every built-in provider kind', async () => {
    await i18next.changeLanguage('en')
    const samples: Array<[string, ProviderFieldSpec]> = [
      ['anthropic', field('key')],
      ['anthropic-compat', field('baseUrl')],
      ['claude-cli', field('cliPath', "otomatik (PATH'te ara)")],
      ['codex-cli', field('configDir')],
      ['deepseek', field('baseUrl')],
      ['deepseek-anthropic', field('key')],
      ['minimax', field('baseUrl', 'varsayilan MiniMax uc noktasi')],
      ['minimax-anthropic', field('key')],
      ['openai-compat', field('baseUrl')],
      ['openrouter', field('baseUrl')],
      ['zai', field('baseUrl')],
      ['lmstudio', field('key')],
    ]

    for (const [providerId, sourceField] of samples) {
      const localized = localizeProviderKind({
        id: providerId,
        label: providerId,
        transport: providerId.endsWith('-cli') ? 'cli' : 'api',
        multi: true,
        fields: [sourceField],
      }).fields[0]

      expect(localized.label, `${providerId} field label`).not.toBe(sourceField.label)
      expect(localized.help, `${providerId} field help`).not.toBe(sourceField.help)
      if (sourceField.placeholder) {
        expect(localized.placeholder, `${providerId} field placeholder`).not.toBe(
          sourceField.placeholder,
        )
      }
    }

    const lmStudio = localizeProviderKind({
      id: 'lmstudio',
      label: 'LM Studio (yerel)',
      transport: 'api',
      multi: true,
      fields: [field('key')],
    })
    expect(lmStudio.fields[0].label).toBe('API key (optional)')
  })

  it('covers every model declared by each static built-in provider kind', async () => {
    await i18next.changeLanguage('en')
    const providers = readStaticProviderModels()

    expect([...providers.keys()].sort()).toEqual([
      'anthropic',
      'claude-cli',
      'codex-cli',
      'deepseek',
      'deepseek-anthropic',
      'lmstudio',
      'minimax',
      'minimax-anthropic',
      'openrouter',
      'zai',
    ])

    for (const [providerId, models] of providers) {
      const localized = localizeCatalogEntry(
        catalogEntry({ id: providerId, label: providerId, models }),
      )
      for (const model of localized.models) {
        expect(
          Object.getOwnPropertyDescriptor(model, 'label')?.get,
          `${providerId}/${model.id} label`,
        ).toBeTypeOf('function')
        expect(
          Object.getOwnPropertyDescriptor(model, 'description')?.get,
          `${providerId}/${model.id} description`,
        ).toBeTypeOf('function')
        expect(model.label).not.toMatch(/^models\./)
        expect(model.description).not.toMatch(/^models\./)
        expect(
          `${model.label} ${model.description}`,
          `${providerId}/${model.id} English copy`,
        ).not.toMatch(turkishCopy)
      }
    }
  })
})

function field(key: string, placeholder?: string): ProviderFieldSpec {
  return {
    key,
    label: 'Alan etiketi',
    type: 'text',
    required: false,
    secret: false,
    help: 'Alan yardımı',
    placeholder,
  }
}
