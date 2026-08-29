import { useState } from 'react'
import { ArrowRight } from 'lucide-react'
import type { TurnStep } from '@/types'
import { STEP_KIND_MAP } from '@/shared/stepKinds'

const HeaderIcon = STEP_KIND_MAP.compaction.Icon

interface Props {
  step: TurnStep
}

// Human label per compaction trigger. An unknown value is rendered verbatim
// rather than hidden, so a new backend trigger shows up instead of vanishing.
const TRIGGER_LABEL: Record<string, string> = {
  auto: 'bütçe eşiği',
  manual: '/compact',
  reactive: 'taşma kurtarması',
}

// CompactionCard reports that the session's history was folded into the rolling
// summary to stay inside the context budget. It renders from the structural
// fields (folded message count, before/after context size, trigger) when the
// step carries them; traces persisted before those fields existed only have
// `text`, so the card falls back to that headline instead of rendering empty.
export function CompactionCard({ step }: Props) {
  const [open, setOpen] = useState(false)
  const { foldedMsgs, beforeTokens, afterTokens, trigger } = step
  const fallback = step.text?.trim()
  const headline = foldedMsgs
    ? `Bağlam sıkıştırıldı — ${foldedMsgs} mesaj özete katlandı`
    : (fallback ?? 'Bağlam sıkıştırıldı')
  // Expandable only when there is something beyond the collapsed row: the
  // original headline (when structural fields produced their own) or the trigger.
  const detail = foldedMsgs && fallback && fallback !== headline ? fallback : undefined
  const expandable = !!detail || !!trigger
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
            title="Katlama öncesi → sonrası bağlam büyüklüğü"
            className="flex shrink-0 items-center gap-1 font-mono text-[10px] opacity-80"
          >
            {fmtTok(beforeTokens!)}
            <ArrowRight size={10} />
            {fmtTok(afterTokens!)} tok
          </span>
        )}
      </button>
      {open && expandable && (
        <div className="border-t border-[color-mix(in_srgb,var(--color-accent)_18%,transparent)] px-3 py-2 text-[11px] text-[var(--color-text-dim)]">
          {detail && <p>{detail}</p>}
          {trigger && (
            <p className={detail ? 'mt-1.5' : undefined}>
              Tetikleyici: <span className="font-mono">{TRIGGER_LABEL[trigger] ?? trigger}</span>
            </p>
          )}
        </div>
      )}
    </div>
  )
}

function fmtTok(n: number): string {
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`
  return `${n}`
}
