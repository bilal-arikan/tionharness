import { useTranslation } from 'react-i18next'
import { Badge } from '@/shared/components'
import { formatTime } from '@/shared/lib/intl'
import { formatBytes, percent, usd } from '@/shared/lib/format'
import type { DebugTrace } from './debugModel'

export function DeciderDebugTrace({ trace }: { trace: DebugTrace }) {
  const { t } = useTranslation('decider')
  const first = trace.events[0]
  const outcome = trace.outcome
  return (
    <details className="rounded border border-[var(--color-border)] p-3 text-xs">
      <summary className="cursor-pointer space-x-2">
        <span>{formatTime(trace.at)}</span>
        <span className="font-medium">
          {t(`authority.${trace.authority}.label`, { defaultValue: trace.authority })}
        </span>
        <Badge tone="muted">{t(`modeLabel.${first.mode}`)}</Badge>
        <Badge tone={trace.completed?.error ? 'danger' : 'muted'}>
          {outcome?.outcome || trace.completed?.error || t('debug.pending')}
        </Badge>
        {outcome?.applied && <Badge tone="success">{t('recent.applied')}</Badge>}
        <span className="text-[var(--color-text-dim)]">{first.sessionId || first.ref}</span>
      </summary>
      <div className="mt-3 space-y-3">
        <p className="break-all text-[var(--color-text-dim)]">
          {t('debug.trace')}: {trace.id} · {t('debug.threshold')}: {percent(first.threshold)}
          {first.turnId && ` · ${t('debug.turn')}: ${first.turnId}`}
          {first.ref && ` · ${t('debug.reference')}: ${first.ref}`}
        </p>
        {outcome?.baseline && (
          <p>{t('debug.comparison', { outcome: outcome.outcome, baseline: outcome.baseline })}</p>
        )}
        <ol className="space-y-2 border-s border-[var(--color-border)] ps-3">
          {trace.events.map((e, i) => (
            <li key={`${e.at}-${i}`} className="space-y-1">
              <p className="flex flex-wrap items-center gap-2">
                <span className="text-[var(--color-text-dim)]">{formatTime(e.at)}</span>
                <span className="font-medium">
                  {t(`debug.stage.${e.stage}`, { defaultValue: e.stage })}
                </span>
                {e.role && <span>{t(`debug.role.${e.role}`, { defaultValue: e.role })}</span>}
                {e.instance && <code>{e.instance}</code>}
                {e.latencyMs !== undefined && (
                  <span>{t('debug.duration', { ms: e.latencyMs })}</span>
                )}
                {e.httpAttempt && <span>{t('debug.attempt', { n: e.httpAttempt })}</span>}
                {e.retryWaitMs ? <span>{t('debug.retryWait', { ms: e.retryWaitMs })}</span> : null}
                {e.error && <Badge tone="danger">{e.error}</Badge>}
              </p>
              {e.timeoutMs !== undefined && (
                <p>{t('debug.budget', { ms: e.timeoutMs, tokens: e.contextTokens ?? 0 })}</p>
              )}
              {e.questionId && (
                <p>
                  {t('debug.question')}: {e.questionId}
                </p>
              )}
              {e.model && (
                <p>
                  {e.model}
                  {e.servedModel && ` → ${e.servedModel}`}
                </p>
              )}
              {e.requestHash && (
                <p className="break-all text-[var(--color-text-dim)]">
                  {t('debug.input', {
                    before: formatBytes(e.stateBytes ?? 0),
                    after: formatBytes(e.preparedBytes ?? 0),
                    n: Object.values(e.questionTypes ?? {}).reduce((sum, count) => sum + count, 0),
                  })}
                  {e.stateTrimmed && ` · ${t('debug.trimmed')}`}
                  <br />
                  {t('debug.fingerprint')}: {e.requestHash}
                </p>
              )}
              {e.answers && (
                <ul className="space-y-1">
                  {Object.entries(e.answers).map(([key, answer]) => (
                    <li key={key}>
                      <code>{key}</code>:{' '}
                      {answer.type === 'noul'
                        ? t('debug.probability', { p: percent(answer.probability ?? 0) })
                        : answer.type === 'choice'
                          ? `${answer.choice} · ${t('debug.confidence', { p: percent(answer.confidence ?? 0) })}`
                          : `${answer.score ?? 0} · ${t('debug.confidence', { p: percent(answer.confidence ?? 0) })}`}
                      {answer.probabilities && (
                        <pre className="overflow-auto text-[var(--color-text-dim)]">
                          {JSON.stringify(answer.probabilities, null, 2)}
                        </pre>
                      )}
                    </li>
                  ))}
                </ul>
              )}
              {e.warnings?.map((warning) => (
                <p key={warning} className="text-[var(--color-warning)]">
                  {t(`debug.warning.${warning}`, { defaultValue: warning })}
                </p>
              ))}
              {e.costUsd !== undefined && (
                <p className="text-[var(--color-text-dim)]">
                  {usd(e.costUsd)} ·{' '}
                  {t('debug.tokens', { input: e.inputTokens ?? 0, output: e.outputTokens ?? 0 })}
                </p>
              )}
            </li>
          ))}
        </ol>
        <p className="break-all text-[var(--color-text-dim)]">
          {t('debug.configFingerprint')}: {first.configHash}
        </p>
      </div>
    </details>
  )
}
