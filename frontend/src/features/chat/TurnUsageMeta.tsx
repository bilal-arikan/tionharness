import type { MessageUsage } from '@/types'
import { tokens as fmtTok } from '@/shared/lib/format'
import { modelDisplayName } from '@/shared/lib/modelLabel'
import { useTranslation } from 'react-i18next'
import { CacheWarmthDot } from './MessageMeta'

const UP_ARROW = String.fromCharCode(0x2191)
const DOWN_ARROW = String.fromCharCode(0x2193)

interface Props {
  model?: string
  usage?: MessageUsage
}

// usageSummary spells the turn's token spend out for the tooltip, e.g.
// "↑32 ↓3.3k · cache 818.3k".
function usageSummary(usage: MessageUsage, cacheLabel: string): string {
  const parts = [`${UP_ARROW}${fmtTok(usage.in)} ${DOWN_ARROW}${fmtTok(usage.out)}`]
  if (usage.cacheRead) parts.push(`${cacheLabel} ${fmtTok(usage.cacheRead)}`)
  return parts.join(' · ')
}

// TurnUsageMeta is the per-turn metadata in a message footer. Only the model
// name is printed: it is what people scan a transcript for. The token counts
// (input / output / cache read) live in the tooltip, and the warm/cold dot
// stays so a cold turn is still spotted without hovering.
export function TurnUsageMeta({ model, usage }: Props) {
  const { t } = useTranslation('chat')
  if (!model && !usage) return null
  const detail = [
    t('assistant.modelUsage'),
    model,
    usage ? usageSummary(usage, t('assistant.cacheRead')) : '',
  ]
    .filter(Boolean)
    .join('\n')
  return (
    <span
      className="flex items-center gap-1.5 text-[10px] text-[var(--color-text-dim)]"
      title={detail}
      data-testid="turn-usage-meta"
    >
      {/* Named, not mono: this is the model people talk about. The exact id
          and the token counts live in the title. */}
      {model && <span className="opacity-80">{modelDisplayName(model)}</span>}
      {/* Warm/cold at a glance, so scanning a transcript shows WHICH turns paid
          a cold prefix without opening each debug panel. Derived from the usage
          already persisted on the message — no fetch. */}
      <CacheWarmthDot usage={usage} />
    </span>
  )
}
