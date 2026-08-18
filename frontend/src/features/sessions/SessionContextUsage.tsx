import type { SessionInfo } from '@/types'
import { Section } from './SessionDetailBits'
import { formatTokens, pctOf, fillerColor } from './sessionDetailFormat'

interface Props {
  info: SessionInfo
  ctxWindow: number
  ctxUsed: number
  ctxFree: number
  ctxPct: number
}

// Context window usage (/context-style)
export function SessionContextUsage({ info, ctxWindow, ctxUsed, ctxFree, ctxPct }: Props) {
  return (
    <Section
      title={`Bağlam penceresi · ${formatTokens(ctxUsed)}/${formatTokens(ctxWindow)} (${ctxPct}%)`}
    >
      {/* Stacked usage bar: each filler a coloured segment, remainder free. */}
      <div className="flex h-2.5 w-full overflow-hidden rounded-full bg-[var(--color-bg)]">
        {info.fillers.map((f) => (
          <div
            key={f.role}
            title={`${f.label}: ~${formatTokens(f.tokens)}`}
            style={{
              width: `${(f.tokens / ctxWindow) * 100}%`,
              backgroundColor: fillerColor(f.role),
            }}
          />
        ))}
      </div>

      <div className="mt-2.5 flex flex-col gap-1">
        {info.fillers.map((f) => (
          <div key={f.role} className="flex items-center gap-2 text-[11px]">
            <span
              className="h-2.5 w-2.5 shrink-0 rounded-sm"
              style={{ backgroundColor: fillerColor(f.role) }}
            />
            <span className="min-w-0 flex-1 truncate text-[var(--color-text)]">
              {f.label}
              {f.count > 1 && <span className="text-[var(--color-text-dim)]"> ·{f.count}</span>}
            </span>
            <span className="shrink-0 font-mono text-[var(--color-text-dim)]">
              ~{formatTokens(f.tokens)} · {pctOf(f.tokens, ctxWindow)}%
            </span>
          </div>
        ))}
        {/* Free space */}
        <div className="flex items-center gap-2 text-[11px]">
          <span className="h-2.5 w-2.5 shrink-0 rounded-sm border border-[var(--color-border)] bg-[var(--color-bg)]" />
          <span className="min-w-0 flex-1 text-[var(--color-text-dim)]">Boş alan</span>
          <span className="shrink-0 font-mono text-[var(--color-text-dim)]">
            ~{formatTokens(ctxFree)} · {pctOf(ctxFree, ctxWindow)}%
          </span>
        </div>
      </div>

      {info.hasSummary && (
        <p className="mt-2 text-[10px] text-[var(--color-text-dim)]">
          İlk {info.summaryMsgCount} mesaj özete katlandı (~{formatTokens(info.summaryTokens)}{' '}
          token).
        </p>
      )}
      {ctxUsed > ctxWindow && (
        <p className="mt-1 text-[10px] text-[var(--color-warning)]">
          Pencere aşıldı — sonraki turda eski turlar özete sıkıştırılır.
        </p>
      )}
    </Section>
  )
}
