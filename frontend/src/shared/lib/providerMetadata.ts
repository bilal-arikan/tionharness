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
  anthropic: { source: 'Anthropic API', key: 'providers.anthropic' },
  minimax: { source: 'MiniMax (OpenAI-uyumlu)', key: 'providers.minimax' },
  'minimax-anthropic': {
    source: 'MiniMax (Anthropic modu · tool-use + thinking)',
    key: 'providers.minimaxAnthropic',
  },
  openrouter: { source: 'OpenRouter (OpenAI-uyumlu)', key: 'providers.openRouter' },
  zai: {
    source: 'Z.ai GLM (Anthropic modu · tool-use + thinking)',
    key: 'providers.zai',
  },
  deepseek: { source: 'DeepSeek (OpenAI-uyumlu)', key: 'providers.deepSeek' },
  'deepseek-anthropic': {
    source: 'DeepSeek (Anthropic modu · tool-use + thinking)',
    key: 'providers.deepSeekAnthropic',
  },
  'codex-cli': {
    source: 'Codex CLI (abonelik · anahtarsız)',
    key: 'providers.codexCli',
  },
  'openai-compat': {
    source: 'OpenAI-uyumlu (özel)',
    key: 'providers.openAICompatible',
  },
  'anthropic-compat': {
    source: 'Anthropic-uyumlu (özel)',
    key: 'providers.anthropicCompatible',
  },
  lmstudio: { source: 'LM Studio (yerel)', key: 'providers.lmStudio' },
}

const providerLabelAliases: Record<string, string[]> = {
  'minimax-anthropic': ['MiniMax (Anthropic modu)'],
  'deepseek-anthropic': ['DeepSeek (Anthropic modu)'],
}

const modelMetadata: Record<string, Record<string, ModelMetadata>> = {
  anthropic: {
    'claude-fable-5-1': model(
      'anthropic.fable51',
      'Claude Fable 5.1 — öncü',
      "En yetenekli genel-erişim model; 1M bağlam, 128K çıktı, adaptif düşünme (daima açık), cache-read Opus'un yarısı ($0.25/MTok). Fable 5 ile aynı fiyat. Farklar: zorunlu tool_choice yok, thinking blokları konuşmaya bağlı (TionHarness drop_block ile tolere eder). 30 günlük veri saklama gerektirir; güvenlik reddi Opus 4.8 fallback'iyle karşılanır (Ayarlar)",
    ),
    'claude-fable-5': model(
      'anthropic.fable5',
      'Claude Fable 5 — önceki öncü',
      "Fable 5.1'in öncülü; 1M bağlam, adaptif düşünme (daima açık), ajan görevleri. Not: 30 günlük veri saklama gerektirir (ZDR organizasyonlarda çalışmaz); güvenlik sınıflandırıcıları reddi Opus 4.8 fallback'iyle karşılanır (Ayarlar)",
    ),
    'claude-opus-5': model(
      'anthropic.opus5',
      'Claude Opus 5 — en yetenekli',
      'En yeni Opus amiral (24 Tem 2026); 1M bağlam, öncü ajan/kodlama + computer-use, Opus fiyatı sabit ($5/$25)',
    ),
    'claude-opus-4-8': model(
      'anthropic.opus48',
      'Claude Opus 4.8 — önceki nesil',
      'Önceki Opus amiral; karmaşık akıl yürütme, kodlama, ajan görevleri',
    ),
    'claude-sonnet-5': model(
      'anthropic.sonnet5',
      'Claude Sonnet 5 — dengeli',
      'En yeni dengeli nesil; 1M bağlam, güçlü ajan/kodlama, hız/kalite dengesi',
    ),
    'claude-sonnet-4-6': model(
      'anthropic.sonnet46',
      'Claude Sonnet 4.6 — önceki dengeli',
      'Güçlü ve hızlı; önceki nesil dengeli model',
    ),
    'claude-haiku-4-5-20251001': model(
      'anthropic.haiku45',
      'Claude Haiku 4.5 — hızlı/ucuz',
      'Düşük gecikme, yüksek hacim, basit görevler',
    ),
  },
  'claude-cli': {
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
  deepseek: {
    'deepseek-flash': model(
      'deepSeek.flash',
      'DeepSeek V4.1 Flash — hızlı/ucuz',
      '1M bağlam; düşünme kapalı ya da low/high/max ($0.15/$0.60 · 1M, cache-hit $0.003; peak saatlerde 2×)',
    ),
    'deepseek-v4-pro': model(
      'deepSeek.v4Pro',
      'DeepSeek V4 Pro — güçlü/akıl-yürütme',
      'V4-Pro-0813 · 1M bağlam; düşünme kapalı ya da low/high/max ($0.66/$1.98 · 1M, cache-hit $0.022; peak saatlerde 2×)',
    ),
    'deepseek-v4-flash': model(
      'deepSeek.v4Flash',
      'DeepSeek V4 Flash — eski ad',
      "Geçici takma ad: DeepSeek bunu V4.1 Flash'a yönlendiriyor (Flash fiyatı); yeni ajanlarda deepseek-flash seçin",
    ),
  },
  'deepseek-anthropic': {
    'deepseek-flash': model(
      'deepSeek.flash',
      'DeepSeek V4.1 Flash — hızlı/ucuz',
      '1M bağlam; düşünme kapalı ya da low/high/max ($0.15/$0.60 · 1M, cache-hit $0.003; peak saatlerde 2×)',
    ),
    'deepseek-v4-pro': model(
      'deepSeek.v4Pro',
      'DeepSeek V4 Pro — güçlü/akıl-yürütme',
      'V4-Pro-0813 · 1M bağlam; düşünme kapalı ya da low/high/max ($0.66/$1.98 · 1M, cache-hit $0.022; peak saatlerde 2×)',
    ),
    'deepseek-v4-flash': model(
      'deepSeek.v4Flash',
      'DeepSeek V4 Flash — eski ad',
      "Geçici takma ad: DeepSeek bunu V4.1 Flash'a yönlendiriyor (Flash fiyatı); yeni ajanlarda deepseek-flash seçin",
    ),
  },
  minimax: {
    'MiniMax-M3': model(
      'minimax.m3',
      'MiniMax M3 - guncel amiral',
      'En yeni akil yurutme/kodlama modeli (varsayilan)',
    ),
    'MiniMax-M2.7': model(
      'minimax.m27',
      'MiniMax M2.7 - onceki nesil',
      'Bir onceki hosted akil-yurutme modeli',
    ),
    'MiniMax-M2.7-highspeed': model(
      'minimax.m27Highspeed',
      'MiniMax M2.7 Highspeed - hizli',
      "M2.7'nin hizli katmani",
    ),
    'MiniMax-M2.5': model('minimax.m25', 'MiniMax M2.5', 'Dengeli akil yurutme katmani'),
    'MiniMax-M2.5-highspeed': model(
      'minimax.m25Highspeed',
      'MiniMax M2.5 Highspeed - hizli',
      "M2.5'in hizli katmani",
    ),
    'MiniMax-M2.1': model(
      'minimax.m21',
      'MiniMax M2.1 — çok dilli kodlama',
      'Güçlü kodlama, çok dil (~60 tps)',
    ),
    'MiniMax-M2.1-lightning': model(
      'minimax.m21Lightning',
      'MiniMax M2.1 Lightning — hızlı',
      "M2.1'in hızlı varyantı (~100 tps)",
    ),
    'MiniMax-M2': model(
      'minimax.m2',
      'MiniMax M2 — ajan/akıl yürütme',
      'Ajan yetenekleri + gelişmiş akıl yürütme',
    ),
  },
  'minimax-anthropic': {
    'MiniMax-M3': model(
      'minimaxAnthropic.m3',
      'MiniMax M3 - guncel amiral',
      'Anthropic modu: arac kullanimi + dusunme',
    ),
    'MiniMax-M2.7': model(
      'minimaxAnthropic.m27',
      'MiniMax M2.7 - onceki nesil',
      'Anthropic modu: onceki hosted akil-yurutme',
    ),
    'MiniMax-M2.5': model(
      'minimaxAnthropic.m25',
      'MiniMax M2.5',
      'Anthropic modu: dengeli akil yurutme',
    ),
    'MiniMax-M2.1': model(
      'minimaxAnthropic.m21',
      'MiniMax M2.1 — çok dilli kodlama',
      'Anthropic modu: araç kullanımı + düşünme',
    ),
    'MiniMax-M2.1-lightning': model(
      'minimaxAnthropic.m21Lightning',
      'MiniMax M2.1 Lightning — hızlı',
      "M2.1'in hızlı varyantı",
    ),
    'MiniMax-M2': model(
      'minimaxAnthropic.m2',
      'MiniMax M2 — ajan/akıl yürütme',
      'Ajan yetenekleri + gelişmiş akıl yürütme',
    ),
  },
  openrouter: {
    'anthropic/claude-fable-5.1': model(
      'openRouter.claudeFable51',
      'Claude Fable 5.1 — öncü',
      'En yeni Anthropic nesli; 1M bağlam, adaptif düşünme (daima açık), ucuz cache-read',
    ),
    'anthropic/claude-fable-5': model(
      'openRouter.claudeFable5',
      'Claude Fable 5 — önceki öncü',
      "Fable 5.1'in öncülü; 1M bağlam, adaptif düşünme (daima açık)",
    ),
    'anthropic/claude-opus-5': model(
      'openRouter.claudeOpus5',
      'Claude Opus 5 — en yetenekli',
      'En yeni Anthropic amiral; 1M bağlam, öncü ajan/kodlama + computer-use',
    ),
    'anthropic/claude-opus-4.8': model(
      'openRouter.claudeOpus48',
      'Claude Opus 4.8 — önceki nesil',
      'Önceki Anthropic amiral; karmaşık akıl yürütme + ajan',
    ),
    'anthropic/claude-opus-4.8-fast': model(
      'openRouter.claudeOpus48Fast',
      'Claude Opus 4.8 (Fast)',
      "Opus 4.8'in hızlı varyantı",
    ),
    'anthropic/claude-opus-4.7': model(
      'openRouter.claudeOpus47',
      'Claude Opus 4.7',
      'Önceki Opus nesli',
    ),
    'anthropic/claude-sonnet-5': model(
      'openRouter.claudeSonnet5',
      'Claude Sonnet 5 — dengeli',
      'En yeni Anthropic dengeli nesli; 1M bağlam, güçlü ajan/kodlama',
    ),
    'anthropic/claude-sonnet-4.6': model(
      'openRouter.claudeSonnet46',
      'Claude Sonnet 4.6 — önceki dengeli',
      'Güçlü ve hızlı; önceki nesil dengeli model',
    ),
    'anthropic/claude-haiku-4.5': model(
      'openRouter.claudeHaiku45',
      'Claude Haiku 4.5 — hızlı/ucuz',
      'Düşük gecikme, yüksek hacim',
    ),
    'openai/gpt-5.5': model('openRouter.gpt55', 'GPT-5.5', 'OpenAI amiral genel-amaçlı'),
    'openai/gpt-5.5-pro': model(
      'openRouter.gpt55Pro',
      'GPT-5.5 Pro',
      "GPT-5.5'in en güçlü katmanı",
    ),
    'google/gemini-3.5-flash': model(
      'openRouter.gemini35Flash',
      'Gemini 3.5 Flash — hızlı',
      'Google hızlı çok-kipli model',
    ),
    'google/gemini-3.1-flash-lite': model(
      'openRouter.gemini31FlashLite',
      'Gemini 3.1 Flash Lite',
      'Çok hızlı/ucuz Google katmanı',
    ),
    'deepseek/deepseek-v4.1-flash': model(
      'openRouter.deepSeekV41Flash',
      'DeepSeek V4.1 Flash',
      'Yeni DeepSeek Flash (2026-09); 1M bağlam, yüksek hacim lideri',
    ),
    'deepseek/deepseek-v4-flash': model(
      'openRouter.deepSeekV4Flash',
      'DeepSeek V4 Flash',
      'Önceki Flash nesli (açık ağırlık)',
    ),
    'deepseek/deepseek-v4-pro': model(
      'openRouter.deepSeekV4Pro',
      'DeepSeek V4 Pro',
      "DeepSeek'in güçlü katmanı",
    ),
    'x-ai/grok-4.3': model('openRouter.grok43', 'Grok 4.3', 'xAI amiral model'),
    'minimax/minimax-m3': model(
      'openRouter.minimaxM3',
      'MiniMax M3',
      "MiniMax'in güncel akıl-yürütme/kodlama modeli",
    ),
    'moonshotai/kimi-k2.6': model(
      'openRouter.kimiK26',
      'Kimi K2.6',
      'Moonshot; geliştiriciler arası saygın',
    ),
    'qwen/qwen3.7-max': model(
      'openRouter.qwen37Max',
      'Qwen3.7 Max',
      'Alibaba en güçlü Qwen katmanı',
    ),
    'qwen/qwen3.7-plus': model('openRouter.qwen37Plus', 'Qwen3.7 Plus', 'Dengeli Qwen katmanı'),
    'nvidia/nemotron-3-ultra-550b-a3b': model(
      'openRouter.nemotron3Ultra',
      'Nemotron 3 Ultra',
      'NVIDIA 550B hibrit MoE',
    ),
    'stepfun/step-3.7-flash': model(
      'openRouter.step37Flash',
      'Step 3.7 Flash',
      'StepFun hızlı kodlama modeli',
    ),
    'z-ai/glm-5.3': model(
      'openRouter.glm53',
      'GLM 5.3',
      'Z.ai amiral model (2026-08); daima düşünür',
    ),
    'z-ai/glm-5.3-flash': model(
      'openRouter.glm53Flash',
      'GLM 5.3 Flash',
      'Z.ai hızlı/ucuz çok-kipli katman',
    ),
    'z-ai/glm-5.2': model('openRouter.glm52', 'GLM 5.2', 'Önceki Z.ai amiral'),
    'mistralai/mistral-medium-3.5': model(
      'openRouter.mistralMedium35',
      'Mistral Medium 3.5',
      'Mistral dengeli model',
    ),
    'tencent/hy3-preview': model(
      'openRouter.tencentHy3',
      'Tencent Hy3 (preview)',
      'Tencent Hunyuan 3 önizleme',
    ),
    'xiaomi/mimo-v2.5': model('openRouter.mimoV25', 'MiMo V2.5', 'Xiaomi popüler kodlama modeli'),
    'openrouter/owl-alpha': model('openRouter.owlAlpha', 'Owl Alpha', 'OpenRouter topluluk modeli'),
    'anthropic/claude-opus-4.7-fast': model(
      'openRouter.claudeOpus47Fast',
      'Claude Opus 4.7 (Fast)',
      "Opus 4.7'nin hızlı varyantı",
    ),
    'google/gemini-3-pro-image': model(
      'openRouter.gemini3ProImage',
      'Gemini 3 Pro (çok-kipli)',
      'Google Gemini 3 Pro — görsel + metin',
    ),
  },
  lmstudio: {
    'qwen3-coder-30b-a3b-instruct': model(
      'lmStudio.qwen3Coder30b',
      'Qwen3 Coder 30B A3B — yerel, kod',
      '256K baglam, MoE (3B aktif); yerelde guvenilir tool-calling icin onerilen sinif. Ucretsiz.',
    ),
    'qwen3-32b': model(
      'lmStudio.qwen332b',
      'Qwen3 32B — yerel, genel',
      '128K baglam, yogun model; genel ajan islerinde guclu. Ucretsiz.',
    ),
    'qwen3-8b': model(
      'lmStudio.qwen38b',
      'Qwen3 8B — yerel, hafif',
      '128K baglam; hizli ve az bellek ister, ancak tool-calling guvenilirligi dusuktur — ozetleyici/alt-ajan rolu icin uygundur. Ucretsiz.',
    ),
  },
  zai: {
    'glm-5.3': model(
      'zai.glm53',
      'GLM-5.3 — güncel amiral',
      'Anthropic modu: araç kullanımı + düşünme (daima açık, low/high/max) ($1.40/$4.40 · 1M, cache $0.26)',
    ),
    'glm-5.3-flash': model(
      'zai.glm53Flash',
      'GLM-5.3 Flash — hızlı/ucuz, çok-kipli',
      "Daima düşünür; Coding Plan'da 3× kota ($0.15/$0.50 · 1M, cache $0.03)",
    ),
    'glm-5.3-flashx': model(
      'zai.glm53FlashX',
      'GLM-5.3 FlashX — en hızlı',
      "Flash'ın ~200 token/sn katmanı; henüz Coding Plan'da yok ($0.37/$1.25 · 1M, cache $0.075)",
    ),
    'glm-5.2': model(
      'zai.glm52',
      'GLM-5.2 — önceki amiral',
      'Anthropic modu: araç kullanımı + düşünme ($1.40/$4.40 · 1M)',
    ),
    'glm-5.1': model(
      'zai.glm51',
      'GLM-5.1 — önceki nesil',
      'Anthropic modu: araç kullanımı + düşünme ($1.40/$4.40 · 1M)',
    ),
    'glm-5': model('zai.glm5', 'GLM-5 — dengeli taban', 'Anthropic modu ($1.00/$3.20 · 1M)'),
    'glm-4.7': model(
      'zai.glm47',
      'GLM-4.7 — önceki kodlama modeli',
      'Anthropic modu: araç kullanımı ($0.60/$2.20 · 1M)',
    ),
    'glm-4.7-flash': model(
      'zai.glm47Flash',
      'GLM-4.7 Flash — ücretsiz',
      "Düşük gecikme, yüksek hacim (Z.ai'de ücretsiz)",
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
}

const fieldHelpKeys: Record<string, Record<string, string>> = {
  anthropic: { key: 'fields.help.anthropic.key' },
  'anthropic-compat': {
    key: 'fields.help.anthropicCompatible.key',
    baseUrl: 'fields.help.anthropicCompatible.baseUrl',
  },
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
  deepseek: { key: 'fields.help.deepSeek.key', baseUrl: 'fields.help.deepSeek.baseUrl' },
  'deepseek-anthropic': { key: 'fields.help.deepSeekAnthropic.key' },
  minimax: { key: 'fields.help.minimax.key', baseUrl: 'fields.help.minimax.baseUrl' },
  'minimax-anthropic': { key: 'fields.help.minimaxAnthropic.key' },
  'openai-compat': {
    key: 'fields.help.openAICompatible.key',
    baseUrl: 'fields.help.openAICompatible.baseUrl',
  },
  openrouter: { key: 'fields.help.openRouter.key', baseUrl: 'fields.help.openRouter.baseUrl' },
  zai: { key: 'fields.help.zai.key', baseUrl: 'fields.help.zai.baseUrl' },
  lmstudio: { key: 'fields.help.lmStudio.key', baseUrl: 'fields.help.lmStudio.baseUrl' },
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
  const labelKey =
    providerId === 'lmstudio' && source.key === 'key'
      ? 'fields.labels.optionalApiKey'
      : fieldLabelKeys[source.key]
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
  if (
    (source.key === 'cliPath' || (providerId === 'minimax' && source.key === 'baseUrl')) &&
    source.placeholder
  ) {
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
