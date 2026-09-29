// Fire ledger vocabulary (R5): skip reasons → Turkish, outcome glyphs. Shared
// by the automation card's ledger and the workspace-signal toasts.
import type { AutomationFireRecord } from '@/types'
import { i18next } from '@/i18n'

const reason = (key: string) => i18next.t(`fireReasons.${key}`, { ns: 'schedules' })

export const SKIP_REASON_LABEL: Record<string, string> = {
  get archived() {
    return reason('archived')
  },
  get disabled() {
    return reason('disabled')
  },
  get expired() {
    return reason('expired')
  },
  get cooldown() {
    return reason('cooldown')
  },
  get max_iterations() {
    return reason('maxIterations')
  },
  get absolute_backstop() {
    return reason('absoluteBackstop')
  },
  get autonomy_paused() {
    return reason('autonomyPaused')
  },
  get target_missing() {
    return reason('targetMissing')
  },
  get agent_archived() {
    return reason('agentArchived')
  },
  get empty_prompt() {
    return reason('emptyPrompt')
  },
  // Rota watcher that names no automation (agent/automation_trajectory.go).
  get not_found() {
    return reason('notFound')
  },
}

export function fireGlyph(outcome: AutomationFireRecord['outcome']): string {
  switch (outcome) {
    case 'fired':
      return '⚡'
    case 'failed':
      return '✕'
    default:
      return '↷'
  }
}
