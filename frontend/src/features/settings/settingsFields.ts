import type { AppSettings, SettingsPatch } from '@/types'
import type { Cat } from './primitives'

// Only fields edited here participate in drafts. Other screens own the rest.
export const CATEGORY_FIELDS: Partial<Record<Cat, (keyof AppSettings)[]>> = {
  profile: [
    'userName',
    'userTimezone',
    'userCity',
    'userCountry',
    'userNotes',
    'language',
    'uiLanguage',
  ],
  general: ['keepAwake', 'autoTitleEnabled', 'autoTagSessions'],
  providers: [
    'extendedPromptCache',
    'anthropicContextEditing',
    'anthropicNativeToolSearch',
    'anthropicProgrammaticTools',
    'anthropicWebTools',
    'anthropicServerCompaction',
    'anthropicRefusalFallback',
    'autonomousTaskBudgetTokens',
    'enableCliHooks',
    'claudeResume',
    'claudePersistentSession',
    'claudeSysPromptFile',
    'claudeCliToolAllowlist',
    'claudeCliNativeSubagents',
    'auxNativeRouting',
  ],
  context: [
    'maxContextTokens',
    'keepRecentMsgs',
    'contextBudgetCeil',
    'contextBudgetFraction',
    'autoCompactMode',
    'handoffAuto',
    'handoffMaxChain',
    'handoffWriteFile',
    'progressPersist',
    'progressResume',
    'reactiveCompact',
    'maxTokenRetries',
    'reactiveKeepRecent',
    'maxOutputTokens',
    'maxProviderRetries',
  ],
  tools: ['enableShell', 'enableCodeMode'],
  execution: [
    'delegationMaxDepth',
    'delegationMaxCalls',
    'spawnMaxConcurrent',
    'spawnQueueMax',
    'spawnMaxPerTurn',
    'spawnIdleTimeoutMin',
    'flowRunRetention',
    'chatTurnIdleTimeoutMin',
    'codexStdoutIdleSec',
    'idleResumeMax',
    'turnIdleWatchdogMin',
    'shellDefaultTimeoutSec',
    'shellMaxTimeoutSec',
    'maxToolOutputKB',
    'coordinatorMaxWorkers',
    'coordinatorMaxDepth',
    'coordinatorSettleGraceSec',
    'coordinatorStallGuard',
    'coordinatorStallSweepMin',
    'coordinatorStallMaxNudges',
    'coordinatorStallNoteVisible',
    'autonomousConfine',
    'autonomousBootSeq',
    'autonomousAutoContinue',
    'autonomousAutoContinueMax',
  ],
  diagnostics: ['debugJournalEnabled', 'debugJournalCap'],
  backup: ['backupEnabled', 'backupIntervalHours', 'backupRetain', 'backupDir'],
}

export function categoryPatch(cat: Cat, draft: AppSettings, original: AppSettings): SettingsPatch {
  return Object.fromEntries(
    (CATEGORY_FIELDS[cat] ?? [])
      .filter((key) => !Object.is(draft[key], original[key]))
      .map((key) => [key, draft[key]]),
  )
}

export function dirtyCategories(draft: AppSettings | null, original: AppSettings | null): Set<Cat> {
  if (!draft || !original) return new Set()
  return new Set(
    (Object.keys(CATEGORY_FIELDS) as Cat[]).filter(
      (cat) => Object.keys(categoryPatch(cat, draft, original)).length > 0,
    ),
  )
}

// A response must not discard edits in another category or typing during a save.
export function rebaseSettingsDraft(
  current: AppSettings,
  original: AppSettings,
  submitted: SettingsPatch,
  updated: AppSettings,
): AppSettings {
  const pending = Object.fromEntries(
    Object.values(CATEGORY_FIELDS)
      .flat()
      .filter((key) =>
        key in submitted
          ? !Object.is(current[key], submitted[key as keyof SettingsPatch])
          : !Object.is(current[key], original[key]),
      )
      .map((key) => [key, current[key]]),
  )
  return { ...updated, ...pending }
}
