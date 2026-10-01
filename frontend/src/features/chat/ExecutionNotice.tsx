import { useTranslation } from 'react-i18next'
import { CheckCircle2, Info, TriangleAlert } from 'lucide-react'
import type { TurnStep } from '@/types'
import { formatDurationMs } from '@/shared/lib/time'
import { deliveryFiles } from './executionEvidence'

export function ExecutionNotice({
  step,
  onOpenFile,
}: {
  step: TurnStep
  onOpenFile?: (path: string) => void
}) {
  const { t } = useTranslation('chatStatus')
  const interrupted = step.operation === 'cli_interrupted'
  const delivery = step.operation === 'delivery_check'
  const mcp = step.operation === 'mcp_startup'
  const warning =
    interrupted || (delivery && step.status !== 'present') || (mcp && step.status !== 'recovered')
  const Icon = warning ? TriangleAlert : delivery || mcp ? CheckCircle2 : Info
  const title = interrupted
    ? t('executionNotice.interrupted')
    : delivery
      ? step.status === 'present'
        ? t('executionNotice.filesPresent')
        : t('executionNotice.filesMissing')
      : mcp
        ? step.status === 'recovered'
          ? t('executionNotice.connectionRecovered')
          : step.status === 'degraded'
            ? t('executionNotice.connectionDegraded')
            : t('executionNotice.connectionRetry')
        : step.status === 'warm'
          ? t('executionNotice.contextWarm')
          : step.status === 'disabled'
            ? t('executionNotice.contextDisabled')
            : t('executionNotice.contextCold')
  const explanation = interrupted
    ? t('executionNotice.partialSaved')
    : delivery
      ? t('executionNotice.presenceOnly')
      : mcp
        ? t('executionNotice.connectionDetail')
        : step.reason === 'compacted'
          ? t('executionNotice.compacted')
          : step.reason === 'multiple_responders' || step.reason === 'persona_changed'
            ? t('executionNotice.responderChanged')
            : step.reason === 'thread_unavailable'
              ? t('executionNotice.threadUnavailable')
              : step.status === 'warm'
                ? t('executionNotice.reused')
                : t('executionNotice.newContext')
  const files = delivery ? deliveryFiles(step.output) : []

  return (
    <details
      className="my-1 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] text-xs"
      data-testid="execution-notice"
      data-operation={step.operation}
    >
      <summary
        className={`flex cursor-pointer items-center gap-2 px-3 py-2 ${warning ? 'text-[var(--color-warning)]' : 'text-[var(--color-text-dim)]'}`}
      >
        <Icon size={13} className="shrink-0" />
        <span className="min-w-0 flex-1 font-medium">{title}</span>
        {Boolean(step.durationMs && step.durationMs > 0) && (
          <span className="text-[10px]">{formatDurationMs(step.durationMs!)}</span>
        )}
      </summary>
      <div className="space-y-2 border-t border-[var(--color-border)] px-3 py-2 text-[var(--color-text)]">
        <p>{explanation}</p>
        {files.length > 0 && (
          <ul className="space-y-1">
            {files.map((file) => (
              <li key={file.path} className="flex items-start gap-2 break-all">
                <span
                  aria-label={
                    file.status === 'present'
                      ? t('executionNotice.filePresent')
                      : t('executionNotice.fileMissing')
                  }
                >
                  {file.status === 'present' ? '✓' : '!'}
                </span>
                {file.status === 'present' && onOpenFile ? (
                  <button
                    type="button"
                    className="text-left text-[var(--color-accent)] underline"
                    onClick={() => onOpenFile(file.path)}
                  >
                    {file.path}
                  </button>
                ) : (
                  <span>{file.path}</span>
                )}
              </li>
            ))}
          </ul>
        )}
        {mcp && step.target?.length ? <p className="break-all">{step.target.join(', ')}</p> : null}
        {mcp && step.output && (
          <pre className="max-h-40 overflow-auto whitespace-pre-wrap break-all text-[10px]">
            {step.output}
          </pre>
        )}
      </div>
    </details>
  )
}
