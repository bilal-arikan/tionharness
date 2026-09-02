// Fire ledger vocabulary (R5): skip reasons → Turkish, outcome glyphs. Shared
// by the automation card's ledger and the workspace-signal toasts.
import type { AutomationFireRecord } from '@/types'

export const SKIP_REASON_LABEL: Record<string, string> = {
  archived: 'arşivli',
  disabled: 'kapalı',
  expired: 'süresi doldu',
  cooldown: 'bekleme süresi',
  max_iterations: 'iterasyon tavanı',
  absolute_backstop: 'mutlak tavan',
  autonomy_paused: 'otonomi duraklatıldı',
  target_missing: 'hedef yok',
  empty_prompt: 'boş prompt',
  // Rota watcher that names no automation (agent/automation_trajectory.go).
  not_found: 'otomasyon bulunamadı',
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
