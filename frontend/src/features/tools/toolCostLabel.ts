// Per-turn token cost of a tool group, rendered for the group row.
//
// The backend reports two estimates per group (see internal/tools/toolcost.go):
// what the group costs at its current tiers, and what it would cost with every
// member at the 'full' tier. Both are approximations, so the label stays coarse
// ("~3.1k tok/tur") — it exists to compare groups with each other, not to bill.
import { formatTokens } from '@/features/sessions/sessionDetailFormat'
import { i18next } from '@/i18n'

export interface ToolGroupCost {
  fullTokens?: number
  currentTokens?: number
}

// costLabel renders the badge text, or null when the backend sent no estimate
// (older build) — the row then simply shows no badge rather than a fake "0".
export function costLabel(g: ToolGroupCost): string | null {
  if (g.currentTokens === undefined) return null
  return i18next.t('cost.label', { ns: 'tools', tokens: formatTokens(g.currentTokens) })
}

// costHint explains the badge and, when the group is not already all-full, what
// promoting it would cost instead. The delta is the number the user is actually
// deciding on.
export function costHint(g: ToolGroupCost): string {
  if (g.currentTokens === undefined) return ''
  const current = i18next.t('cost.current', {
    ns: 'tools',
    tokens: formatTokens(g.currentTokens),
  })
  if (g.fullTokens === undefined) return current
  const delta = g.fullTokens - g.currentTokens
  if (delta <= 0) {
    return `${current} ${i18next.t('cost.alreadyFull', { ns: 'tools' })}`
  }
  return `${current} ${i18next.t('cost.promote', {
    ns: 'tools',
    fullTokens: formatTokens(g.fullTokens),
    delta: formatTokens(delta),
  })}`
}
