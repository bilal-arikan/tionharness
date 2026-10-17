import type { CatalogModelInfo, ProviderFieldSpec, ProviderKind } from '@/api/providers'
import type { CatalogEntry, CatalogModel } from '@/types'
import { i18next } from '@/i18n'

const NAMESPACE = 'providerMetadata'

type TextMetadata = { source: string; key: string }
type ModelMetadata = { label?: TextMetadata; description?: TextMetadata }

const providerLabels: Record<string, TextMetadata> = {
  'claude-cli': {
    source: 'Claude CLI (abonelik · anahtarsız)',
    key: 'providers.claudeCli',
  },
  'codex-cli': {
    source: 'Codex CLI (abonelik · anahtarsız)',
    key: 'providers.codexCli',
  },
  'opencode-cli': {
    source: "OpenCode CLI (çoklu sağlayıcı · anahtar opencode'da)",
    key: 'providers.opencodeCli',
  },
}

const providerLabelAliases: Record<string, string[]> = {}

const modelMetadata: Record<string, Record<string, ModelMetadata>> = {
  'claude-cli': {
    'claude-opus-5-5': model(
      'claudeCli.opus55',
      'Claude Opus 5.5',
      'Pinned Opus 5.5 for coding and knowledge work; adaptive thinking always on. Access depends on the configured account.',
    ),
    'claude-sonnet-5-5': model(
      'claudeCli.sonnet55',
      'Claude Sonnet 5.5',
      'Pinned Sonnet 5.5 for balanced speed and intelligence; adaptive thinking. Access depends on the configured account.',
    ),
    '': model(
      'claudeCli.session',
      'claude oturum modeli',
      'claude oturumunun aktif modelini kullanır',
    ),
    fable: model(
      'claudeCli.fable',
      'Fable — öncü',
      "CLI'nin güncel Fable'ı (5.1); 1M bağlam, adaptif düşünme (daima açık)",
    ),
    opus: model('claudeCli.opus', 'Opus — en güçlü', 'En yetenekli; en yavaş/pahalı'),
    sonnet: model('claudeCli.sonnet', 'Sonnet — dengeli', 'Hız/kalite dengesi (günlük kullanım)'),
    haiku: model('claudeCli.haiku', 'Haiku — hızlı', 'En hızlı/ucuz; basit görevler'),
  },
  'codex-cli': {
    '': model(
      'codexCli.session',
      'codex oturum modeli',
      'codex oturumunun aktif modelini kullanır',
    ),
    'gpt-6.1-sol': model(
      'codexCli.gpt61Sol',
      'GPT-6.1 Sol',
      'Near-Astra performance for complex coding at lower cost. Codex effort: low through ultra; verified CLI context: 272K. Availability depends on the signed-in account.',
    ),
    'gpt-6-astra': model(
      'codexCli.gpt6Astra',
      'GPT-6 Astra',
      'Frontier reasoning and coding. Codex effort: low through ultra; verified CLI context: 272K. Availability depends on the signed-in account.',
    ),
    'gpt-6-sol': model(
      'codexCli.gpt6Sol',
      'GPT-6 Sol',
      'Balanced coding and agentic workflows. Codex effort: low through ultra; verified CLI context: 272K. Availability depends on the signed-in account.',
    ),
    'gpt-6-luna': model(
      'codexCli.gpt6Luna',
      'GPT-6 Luna',
      'Efficient focused tasks. Codex effort: low through max; verified CLI context: 272K. Availability depends on the signed-in account.',
    ),
    'gpt-5.6-sol': model(
      'codexCli.gpt56Sol',
      'GPT-5.6 Sol',
      'Previous generation; published API context about 1.05M (long-context pricing above 272K)',
    ),
    'gpt-5.6-terra': model(
      'codexCli.gpt56Terra',
      'GPT-5.6 Terra',
      'Previous generation; published API context about 1.05M (long-context pricing above 272K)',
    ),
    'gpt-5.6-luna': model(
      'codexCli.gpt56Luna',
      'GPT-5.6 Luna',
      'Previous generation; published API context 400K',
    ),
    'gpt-5.5': model('codexCli.gpt55', 'GPT-5.5', 'Codex CLI modeli'),
    'gpt-5.4': model('codexCli.gpt54', 'GPT-5.4', 'ChatGPT hesabıyla kullanılamaz (API faturalı)'),
    'gpt-5.4-mini': model(
      'codexCli.gpt54Mini',
      'GPT-5.4 Mini — hızlı',
      'En hızlı/ucuz; basit görevler',
    ),
    'gpt-5.2': model('codexCli.gpt52', 'GPT-5.2', 'ChatGPT hesabıyla kullanılamaz (API faturalı)'),
  },
  'opencode-cli': {
    '': model(
      'opencodeCli.session',
      'opencode varsayılan modeli',
      'opencode evinin kendi varsayılan modelini kullanır (opencode.json `model`)',
    ),
    'anthropic/claude-sonnet-5-5': model(
      'opencodeCli.anthropicSonnet55',
      'Anthropic · Claude Sonnet 5.5',
      "Anthropic API anahtarı opencode evinde (`opencode auth login`). Abonelik (Pro/Max) girişi opencode'da YASAK; bunun için claude-cli kullanın.",
    ),
    'anthropic/claude-opus-5-5': model(
      'opencodeCli.anthropicOpus55',
      'Anthropic · Claude Opus 5.5',
      'Anthropic API anahtarı ile.',
    ),
    'openai/gpt-5.5': model(
      'opencodeCli.openaiGpt55',
      'OpenAI · GPT-5.5',
      'OpenAI API anahtarı veya ChatGPT girişi ile.',
    ),
    'openrouter/anthropic/claude-sonnet-5-5': model(
      'opencodeCli.openrouterSonnet55',
      'OpenRouter · Claude Sonnet 5.5',
      'OpenRouter anahtarı opencode evinde.',
    ),
    'deepseek/deepseek-chat': model(
      'opencodeCli.deepseekChat',
      'DeepSeek · chat',
      'DeepSeek anahtarı opencode evinde.',
    ),
    'opencode/big-pickle': model(
      'opencodeCli.bigPickle',
      'OpenCode Zen · big-pickle (ücretsiz)',
      'Anahtarsız ücretsiz deneme modeli; kalite düşük, bağlantı testi için.',
    ),
  },
}

const fieldLabelKeys: Record<string, string> = {
  key: 'fields.labels.apiKey',
  baseUrl: 'fields.labels.baseUrl',
  cliPath: 'fields.labels.cliPath',
  configDir: 'fields.labels.configDir',
  authKind: 'fields.labels.authKind',
  authToken: 'fields.labels.authToken',
  effortVariant: 'fields.labels.effortVariant',
}

const fieldHelpKeys: Record<string, Record<string, string>> = {
  'claude-cli': {
    cliPath: 'fields.help.claudeCli.cliPath',
    configDir: 'fields.help.claudeCli.configDir',
    authKind: 'fields.help.claudeCli.authKind',
    authToken: 'fields.help.claudeCli.authToken',
  },
  'codex-cli': {
    cliPath: 'fields.help.codexCli.cliPath',
    configDir: 'fields.help.codexCli.configDir',
  },
  'opencode-cli': {
    cliPath: 'fields.help.opencodeCli.cliPath',
    configDir: 'fields.help.opencodeCli.configDir',
    effortVariant: 'fields.help.opencodeCli.effortVariant',
  },
}

function text(key: string): string {
  return i18next.t(key, { ns: NAMESPACE }) as string
}

function model(key: string, label: string, description: string): ModelMetadata {
  return {
    label: { source: label, key: `models.${key}.label` },
    description: { source: description, key: `models.${key}.description` },
  }
}

function defineLocalizedProperty<T extends object, K extends keyof T>(
  target: T,
  property: K,
  metadata: TextMetadata | undefined,
  original: T[K],
): void {
  if (!metadata || original !== metadata.source) return
  Object.defineProperty(target, property, {
    enumerable: true,
    configurable: true,
    get: () => text(metadata.key),
  })
}

function localizeModel<T extends CatalogModel | CatalogModelInfo>(
  providerId: string,
  source: T,
): T {
  const localized = { ...source } as T
  const metadata = modelMetadata[providerId]?.[source.id]
  defineLocalizedProperty(localized, 'label', metadata?.label, source.label)
  defineLocalizedProperty(localized, 'description', metadata?.description, source.description)
  return localized
}

function localizeField(providerId: string, source: ProviderFieldSpec): ProviderFieldSpec {
  const localized = { ...source }
  const labelKey = fieldLabelKeys[source.key]
  if (labelKey) {
    Object.defineProperty(localized, 'label', {
      enumerable: true,
      configurable: true,
      get: () => text(labelKey),
    })
  }
  const helpKey = fieldHelpKeys[providerId]?.[source.key]
  if (helpKey && source.help) {
    Object.defineProperty(localized, 'help', {
      enumerable: true,
      configurable: true,
      get: () => text(helpKey),
    })
  }
  if (source.key === 'cliPath' && source.placeholder) {
    Object.defineProperty(localized, 'placeholder', {
      enumerable: true,
      configurable: true,
      get: () =>
        text(
          source.key === 'cliPath'
            ? 'fields.placeholders.autoPath'
            : 'fields.placeholders.minimaxEndpoint',
        ),
    })
  }
  return localized
}

/** Localize built-in provider-kind metadata while keeping every machine field unchanged. */
export function localizeProviderKind(source: ProviderKind): ProviderKind {
  if (!providerLabels[source.id]) return source
  const localized: ProviderKind = {
    ...source,
    fields: source.fields.map((field) => localizeField(source.id, field)),
    models: source.models?.map((entry) => localizeModel(source.id, entry)),
  }
  defineLocalizedProperty(
    localized,
    'label',
    providerLabelMetadata(source.id, source.label),
    source.label,
  )
  return localized
}

/** Localize known built-in catalog copy and preserve custom/unknown server metadata verbatim. */
export function localizeCatalogEntry(source: CatalogEntry): CatalogEntry {
  const localized: CatalogEntry = {
    ...source,
    models: source.models.map((entry) => localizeModel(source.id, entry) as CatalogModel),
  }
  defineLocalizedProperty(
    localized,
    'label',
    providerLabelMetadata(source.id, source.label),
    source.label,
  )
  return localized
}

function providerLabelMetadata(providerId: string, source: string): TextMetadata | undefined {
  const metadata = providerLabels[providerId]
  if (!metadata) return undefined
  if (metadata.source === source) return metadata
  return providerLabelAliases[providerId]?.includes(source) ? { ...metadata, source } : undefined
}
