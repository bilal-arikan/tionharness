import type { SessionInfo } from '@/types'
import { KeyValueRow as Row } from '@/shared/components'
import { Section } from './SessionDetailBits'
import { formatTokens } from './sessionDetailFormat'
import { useTranslation } from 'react-i18next'
import { i18next } from '@/i18n'

// eslint-disable-next-line react-refresh/only-export-components
export function formatDurationMs(ms: number): string {
  if (ms <= 0) return '—'
  if (ms < 1000) return `${ms} ms`
  const seconds = Math.round(ms / 100) / 10
  return i18next.t('execution.seconds', { ns: 'sessions', value: seconds })
}

// Safe persisted execution summary. Raw TurnStep payloads stay in the authorized
// transcript/debug surfaces; this card exposes only counts and stable reasons.
export function SessionExecutionCard({ info }: { info: SessionInfo }) {
  const { t } = useTranslation('sessions')
  // The delegation target, named rather than shown as a raw id: the info payload
  // resolves agentName from the target for a delegated run, so prefer it and keep
  // the id only as the fallback for a target that no longer resolves.
  const target = info.targetProfile
    ? t('execution.targetProfile', { profile: info.targetProfile })
    : info.targetAgentId
      ? t('execution.targetAgent', { agent: info.agentName || info.targetAgentId })
      : '—'
  const tokens = t('execution.tokenSummary', {
    input: formatTokens(info.inputTokens ?? 0),
    output: formatTokens(info.outputTokens ?? 0),
  })

  return (
    <Section title={t('execution.title')}>
      <Row
        label={t('execution.typeCategory')}
        value={`${info.executionType ?? '—'} / ${info.category ?? '—'}`}
      />
      <Row label={t('execution.target')} value={target} />
      <Row label={t('execution.context')} value={info.contextMode ?? '—'} />
      <Row
        label={t('execution.state')}
        value={
          info.runState
            ? t(`execution.runState.${info.runState}`, { defaultValue: info.runState })
            : info.terminal
              ? t('execution.terminal')
              : '—'
        }
      />
      <Row label={t('execution.duration')} value={formatDurationMs(info.durationMs ?? 0)} />
      <Row label={t('execution.tokens')} value={tokens} />
      <Row label={t('execution.toolCalls')} value={String(info.toolCallCount ?? 0)} />
      <Row label={t('execution.persistedSteps')} value={String(info.persistedSteps ?? 0)} />
      {info.stopReason && <Row label={t('execution.stopReason')} value={info.stopReason} />}
      {info.errorSummary && <Row label={t('execution.errorSummary')} value={info.errorSummary} />}
    </Section>
  )
}
