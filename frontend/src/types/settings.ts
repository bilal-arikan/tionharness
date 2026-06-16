// Application (global) settings, provider/model catalog and read-only prompts.

// Mirrors settings.DTO — the Anthropic key is never returned; anthropicKeySet
// reports whether one is stored.
export type Theme = 'dark' | 'light' | 'system'

export interface AppSettings {
  theme: Theme
  accent: string
  // Curated palette id (see lib/themePresets). When set, it overrides the full
  // token set; "" falls back to legacy theme + accent behaviour.
  themePreset: string
  language: 'tr' | 'en'

  defaultProvider: string
  defaultModel: string
  defaultPermissionMode: string
  claudeCliPath: string
  anthropicKeySet: boolean
  minimaxKeySet: boolean
  minimaxBaseUrl: string

  oneMillionContext: boolean
  extendedPromptCache: boolean

  desktopNotifications: boolean
  keepAwake: boolean

  userName: string
  userTimezone: string
  userCity: string
  userCountry: string
  userNotes: string

  maxContextTokens: number
  keepRecentMsgs: number
  recallTopN: number
  recallMinScore: number

  journalCap: number
  journalMaxLen: number

  autoReflect: boolean
  autoReflectThreshold: number

  defaultDailyCallLimit: number
  defaultDailyTokenLimit: number

  defaultHeartbeatSec: number
  pauseAutonomy: boolean

  autoTitleEnabled: boolean
  titleModel: string

  mcpGatewayUrl: string

  logLevel: string
}

// Partial update. anthropicKey/minimaxKey are write-only: "" clears, non-empty sets.
export type SettingsPatch = Partial<
  Omit<AppSettings, 'anthropicKeySet' | 'minimaxKeySet'> & {
    anthropicKey: string
    minimaxKey: string
  }
>

export interface ProviderTestResult {
  ok: boolean
  model?: string
  sample?: string
  error?: string
}

// Provider/model catalog for the UI's pickers.
export interface CatalogModel {
  id: string
  label: string
  description?: string
}

export interface CatalogEntry {
  id: string
  label: string
  needsKey: boolean
  allowCustomModel: boolean
  available: boolean
  models: CatalogModel[]
}

// A built-in runtime prompt (summary/reflect/title), shown read-only in the
// Komutlar settings screen.
export interface PromptInfo {
  key: string
  label: string
  file: string
  system: string
  user: string
  note: string
}

export interface PromptsResponse {
  dir: string
  prompts: PromptInfo[]
}
