import { i18next } from '@/i18n'

// Resolve on demand so callers always receive the active interface language.
export function workflowHelp(): string {
  return i18next.t('workflow.help', { ns: 'sharedUi' })
}
