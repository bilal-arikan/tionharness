import type { SessionInfo } from '@/types'
import { Section } from './SessionDetailBits'
import { formatTokens, pctOf } from './sessionDetailFormat'
import { buildContextRows, contextToneColor, type ContextRow } from './contextUsageModel'
import { useTranslation } from 'react-i18next'

const FILLER_LABEL_KEYS: Record<string, string> = {
  summary: 'contextUsage.filler.summary',
  user: 'contextUsage.filler.user',
  assistant: 'contextUsage.filler.assistant',
  tool: 'contextUsage.filler.tool',
  system: 'contextUsage.filler.systemPrompt',
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
  const rows = buildContextRows(info.fillers, ctxWindow)
  const rowLabel = (row: ContextRow) =>
    row.filler ? fillerLabel(row.filler) : t('contextUsage.other')
  // The folded row's tooltip names what it holds, so nothing becomes unreachable.
  const rowTitle = (row: ContextRow) =>
    row.folded
      ? row.folded.map((f) => `${fillerLabel(f)}: ${formatTokens(f.tokens)}`).join('\n')
      : row.calibrated
        ? t('contextUsage.exact')
        : t('contextUsage.estimated')
  const hasEstimate = rows.some((row) => !row.calibrated)
  return (
    <Section
      title={t('contextUsage.title', {
        used: formatTokens(ctxUsed),
        window: formatTokens(ctxWindow),
        percent: ctxPct,
      })}
      info={t('contextUsage.explanation')}
    >
      {/* Stacked usage bar: one segment per row, split by a 1px seam, the
          remainder free. Segment and legend swatch share one colour source. */}
      <div
        data-testid="context-usage-bar"
        className="flex h-2.5 w-full gap-px overflow-hidden rounded-full bg-[var(--color-bg)]"
      >
        {rows.map((row) => (
          <div
            key={row.key}
            data-tone={row.tone}
            title={`${rowLabel(row)}: ${formatTokens(row.tokens)}`}
            style={{
              width: `${(row.tokens / ctxWindow) * 100}%`,
              backgroundColor: contextToneColor(row.tone),
            }}
          />
        ))}
      </div>

      <div className="mt-2.5 flex flex-col gap-1">
        {rows.map((row) => (
          <div
            key={row.key}
            data-testid={`context-usage-row-${row.key}`}
            className="flex items-center gap-2 text-[11px]"
          >
            <span
              className="h-2.5 w-2.5 shrink-0 rounded-sm"
              style={{ backgroundColor: contextToneColor(row.tone) }}
            />
            <span className="min-w-0 flex-1 truncate text-[var(--color-text)]">
              {rowLabel(row)}
              {row.count > 1 && <span className="text-[var(--color-text-dim)]"> ·{row.count}</span>}
            </span>
            <span className="shrink-0 font-mono text-[var(--color-text-dim)]" title={rowTitle(row)}>
              {formatTokens(row.tokens)} · {pctOf(row.tokens, ctxWindow)}%
            </span>
          </div>
        ))}
        {/* Free space */}
        <div className="flex items-center gap-2 text-[11px]">
          <span className="h-2.5 w-2.5 shrink-0 rounded-sm border border-[var(--color-border)] bg-[var(--color-bg)]" />
          <span className="min-w-0 flex-1 text-[var(--color-text-dim)]">
            {t('contextUsage.free')}
          </span>
          <span className="shrink-0 font-mono text-[var(--color-text-dim)]">
            {formatTokens(ctxFree)} · {pctOf(ctxFree, ctxWindow)}%
          </span>
        </div>
      </div>

      {/* One note instead of a "~" on every figure. */}
      {hasEstimate && (
        <p className="mt-2 text-[11px] text-[var(--color-text-dim)]">
          {t('contextUsage.approxNote')}
        </p>
      )}
      {info.hasSummary && (
        <p className="mt-2 text-[11px] text-[var(--color-text-dim)]">
          {t('contextUsage.summarized', {
            count: info.summaryMsgCount,
            tokens: formatTokens(info.summaryTokens),
          })}
        </p>
      )}
      {ctxUsed > ctxWindow && (
        <p className="mt-1 text-[11px] text-[var(--color-warning)]">{t('contextUsage.exceeded')}</p>
      )}
    </Section>
  )
}
