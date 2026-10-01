import type { SessionInfo } from '@/types'
import { Section } from './SessionDetailBits'
import { formatTokens, pctOf, fillerColor } from './sessionDetailFormat'
import { useTranslation } from 'react-i18next'

const FILLER_LABEL_KEYS: Record<string, string> = {
  summary: 'contextUsage.filler.summary',
  user: 'contextUsage.filler.user',
  assistant: 'contextUsage.filler.assistant',
  tool: 'contextUsage.filler.tool',
  system: 'contextUsage.filler.systemPrompt',
  'worker-note': 'contextUsage.filler.workerResults',
  'auto-prompt': 'contextUsage.filler.autoPrompt',
  'tool-history': 'contextUsage.filler.toolHistory',
  skills: 'contextUsage.filler.skillsCatalog',
  'lazy-tools': 'contextUsage.filler.lazyToolsCatalog',
  tools: 'contextUsage.filler.tools',
  'tool-activity': 'contextUsage.filler.toolActivity',
  artifacts: 'contextUsage.filler.artifacts',
}

interface Props {
  info: SessionInfo
  ctxWindow: number
  ctxUsed: number
  ctxFree: number
  ctxPct: number
}

// Context window usage (/context-style)
export function SessionContextUsage({ info, ctxWindow, ctxUsed, ctxFree, ctxPct }: Props) {
  const { t } = useTranslation('sessions')
  const fillerLabel = (filler: SessionInfo['fillers'][number]) => {
    if (filler.role === 'cli-harness') {
      const provider = info.running?.provider
      if (filler.calibrated) {
        return provider
          ? t('contextUsage.filler.cliMeasuredProvider', { provider })
          : t('contextUsage.filler.cliMeasured')
      }
      return provider
        ? t('contextUsage.filler.cliReferenceProvider', { provider })
        : t('contextUsage.filler.cliReference')
    }
    const key = FILLER_LABEL_KEYS[filler.role]
    return key ? t(key) : filler.label
  }
  return (
    <Section
      title={t('contextUsage.title', {
        used: formatTokens(ctxUsed),
        window: formatTokens(ctxWindow),
        percent: ctxPct,
      })}
    >
      <p className="mb-2 text-[10px] text-[var(--color-text-dim)]">
        {t('contextUsage.explanation')}
      </p>
      {/* Stacked usage bar: each filler a coloured segment, remainder free. */}
      <div className="flex h-2.5 w-full overflow-hidden rounded-full bg-[var(--color-bg)]">
        {info.fillers.map((f) => {
          const label = fillerLabel(f)
          return (
            <div
              key={f.role}
              title={`${label}: ~${formatTokens(f.tokens)}`}
              style={{
                width: `${(f.tokens / ctxWindow) * 100}%`,
                backgroundColor: fillerColor(f.role),
              }}
            />
          )
        })}
      </div>

      <div className="mt-2.5 flex flex-col gap-1">
        {info.fillers.map((f) => {
          const label = fillerLabel(f)
          return (
            <div key={f.role} className="flex items-center gap-2 text-[11px]">
              <span
                className="h-2.5 w-2.5 shrink-0 rounded-sm"
                style={{ backgroundColor: fillerColor(f.role) }}
              />
              <span className="min-w-0 flex-1 truncate text-[var(--color-text)]">
                {label}
                {f.count > 1 && <span className="text-[var(--color-text-dim)]"> ·{f.count}</span>}
              </span>
              <span
                className="shrink-0 font-mono text-[var(--color-text-dim)]"
                title={f.calibrated ? t('contextUsage.exact') : t('contextUsage.estimated')}
              >
                {f.calibrated ? '' : '~'}
                {formatTokens(f.tokens)} · {pctOf(f.tokens, ctxWindow)}%
              </span>
            </div>
          )
        })}
        {/* Free space */}
        <div className="flex items-center gap-2 text-[11px]">
          <span className="h-2.5 w-2.5 shrink-0 rounded-sm border border-[var(--color-border)] bg-[var(--color-bg)]" />
          <span className="min-w-0 flex-1 text-[var(--color-text-dim)]">
            {t('contextUsage.free')}
          </span>
          <span className="shrink-0 font-mono text-[var(--color-text-dim)]">
            ~{formatTokens(ctxFree)} · {pctOf(ctxFree, ctxWindow)}%
          </span>
        </div>
      </div>

      {info.hasSummary && (
        <p className="mt-2 text-[10px] text-[var(--color-text-dim)]">
          {t('contextUsage.summarized', {
            count: info.summaryMsgCount,
            tokens: formatTokens(info.summaryTokens),
          })}
        </p>
      )}
      {ctxUsed > ctxWindow && (
        <p className="mt-1 text-[10px] text-[var(--color-warning)]">{t('contextUsage.exceeded')}</p>
      )}
    </Section>
  )
}
