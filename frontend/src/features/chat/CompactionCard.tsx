import { useState } from 'react'
import { tokens as fmtTok } from '@/shared/lib/format'
import { ArrowRight } from 'lucide-react'
import type { TurnStep } from '@/types'
import { STEP_KIND_MAP } from '@/shared/stepKinds'
import { useTranslation } from 'react-i18next'

const HeaderIcon = STEP_KIND_MAP.compaction.Icon

interface Props {
  step: TurnStep
}

// Human label per compaction trigger. An unknown value is rendered verbatim
// rather than hidden, so a new backend trigger shows up instead of vanishing.
const PROVIDER_LABEL: Record<string, string> = {
  'claude-cli': 'Claude CLI',
  'codex-cli': 'Codex CLI',
}

// CompactionCard reports that the session's history was folded into the rolling
// summary to stay inside the context budget. It renders from the structural
// fields (folded message count, before/after context size, trigger) when the
// step carries them; traces persisted before those fields existed only have
// `text`, so the card falls back to that headline instead of rendering empty.
export function CompactionCard({ step }: Props) {
  const { t } = useTranslation('chat')
  const [open, setOpen] = useState(false)
  const { foldedMsgs, beforeTokens, afterTokens, trigger, source, provider, sessionAction } = step
  const fallback = step.text?.trim()
  const headline = step.running
    ? t('compaction.running')
    : foldedMsgs
      ? t('compaction.folded', { count: foldedMsgs })
      : (fallback ?? t('compaction.complete'))
  // Expandable only when there is something beyond the collapsed row: the
  // original headline (when structural fields produced their own), trigger or
  // provenance/action metadata.
  const detail = foldedMsgs && fallback && fallback !== headline ? fallback : undefined
  const expandable = !!detail || !!trigger || !!source || !!provider || !!sessionAction
  const shrink = !!beforeTokens && !!afterTokens

  return (
    <div className="my-0.5 rounded-md border border-[color-mix(in_srgb,var(--color-accent)_30%,transparent)] bg-[color-mix(in_srgb,var(--color-accent)_7%,transparent)] text-xs">
      <button
        type="button"
        onClick={() => expandable && setOpen((v) => !v)}
        className={`flex w-full items-center gap-2 px-3 py-1.5 text-left text-[var(--color-accent)] ${expandable ? '' : 'cursor-default'}`}
      >
        <HeaderIcon size={14} className="shrink-0" />
        <span className="min-w-0 flex-1 truncate font-medium">{headline}</span>
        {shrink && (
          <span
            title={t('compaction.contextSize')}
            className="flex shrink-0 items-center gap-1 font-mono text-[10px] opacity-80"
          >
            {fmtTok(beforeTokens!)}
            <ArrowRight size={10} />
            {t('common.tokenShort', { count: fmtTok(afterTokens!) })}
          </span>
        )}
      </button>
      {open && expandable && (
        <div className="border-t border-[color-mix(in_srgb,var(--color-accent)_18%,transparent)] px-3 py-2 text-[11px] text-[var(--color-text-dim)]">
          {detail && <p>{detail}</p>}
          {trigger && (
            <p className={detail ? 'mt-1.5' : undefined}>
              {t('compaction.trigger')}:{' '}
              <span className="font-mono">
                {t(`compaction.triggerValue.${trigger}`, { defaultValue: trigger })}
              </span>
            </p>
          )}
          {source && (
            <p className={detail || trigger ? 'mt-1.5' : undefined}>
              {t('compaction.source')}:{' '}
              <span className="font-mono">
                {t(`compaction.sourceValue.${source}`, { defaultValue: source })}
              </span>
            </p>
          )}
          {provider && (
            <p className={detail || trigger || source ? 'mt-1.5' : undefined}>
              {t('compaction.provider')}:{' '}
              <span className="font-mono">{PROVIDER_LABEL[provider] ?? provider}</span>
            </p>
          )}
          {sessionAction && (
            <p className={detail || trigger || source || provider ? 'mt-1.5' : undefined}>
              {t('compaction.sessionAction')}:{' '}
              <span className="font-mono">
                {t(`compaction.sessionActionValue.${sessionAction}`, {
                  defaultValue: sessionAction,
                })}
              </span>
            </p>
          )}
        </div>
      )}
    </div>
  )
}
