import type { TurnStep } from '@/types'
import { STEP_KIND_MAP } from '@/shared/stepKinds'

const HeaderIcon = STEP_KIND_MAP.error.Icon

// Reason tags the backend stamps on a terminal usage/rate-limit, overload or
// billing failure (see internal/agent/errclass.go). They read as an actionable
// "hit limit" state — worth a dedicated title + a stronger nudge to the retry
// button — rather than a generic crash. Kept in sync with the errClass strings.
const LIMIT_REASONS: Record<string, string> = {
  rate_limit: 'Kullanım limitine ulaşıldı',
  overloaded: 'Sağlayıcı aşırı yüklü',
  billing: 'Kredi / kota tükendi',
}

interface Props {
  step: TurnStep
}

// ErrorStep renders a turn-level failure (provider error, budget exceeded,
// cancellation) inline — visually stronger than a recovery notice. A recognised
// limit reason gets a bold title and a "Yeniden dene ile sürdürebilirsiniz"
// hint so a hit-limit turn is obviously recoverable, not dead.
export function ErrorStep({ step }: Props) {
  const limitTitle = step.reason ? LIMIT_REASONS[step.reason] : undefined
  return (
    <div className="flex items-start gap-2 rounded-md border border-[color-mix(in_srgb,var(--color-danger)_45%,transparent)] bg-[color-mix(in_srgb,var(--color-danger)_12%,transparent)] px-3 py-1.5 text-xs text-[var(--color-danger)]">
      <HeaderIcon size={14} className="mt-0.5 shrink-0" />
      <div className="min-w-0 flex-1">
        {limitTitle && <div className="mb-0.5 font-semibold">⛔ {limitTitle}</div>}
        <span className="whitespace-pre-wrap break-words">{step.text || 'Hata'}</span>
        {limitTitle && (
          <div className="mt-1 opacity-80">
            Aşağıdaki “Yeniden dene” butonuyla turu sürdürebilirsiniz.
          </div>
        )}
      </div>
      {step.reason && (
        <span className="shrink-0 rounded bg-[color-mix(in_srgb,var(--color-danger)_22%,transparent)] px-1.5 py-0.5 font-mono text-[10px] opacity-80">
          {step.reason}
        </span>
      )}
    </div>
  )
}
