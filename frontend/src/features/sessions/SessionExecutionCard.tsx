import type { SessionInfo } from '@/types'
import { KeyValueRow as Row } from '@/shared/components'
import { Section } from './SessionDetailBits'
import { formatTokens } from './sessionDetailFormat'

const terminalLabel: Record<string, string> = {
  completed: 'Tamamlandı',
  failed: 'Başarısız',
  killed: 'Durduruldu',
  timeout: 'Zaman aşımı',
  incomplete: 'Eksik',
  running: 'Çalışıyor',
}

export function formatDurationMs(ms: number): string {
  if (ms <= 0) return '—'
  if (ms < 1000) return `${ms} ms`
  const seconds = Math.round(ms / 100) / 10
  return `${seconds} sn`
}

// Safe persisted execution summary. Raw TurnStep payloads stay in the authorized
// transcript/debug surfaces; this card exposes only counts and stable reasons.
export function SessionExecutionCard({ info }: { info: SessionInfo }) {
  const target = info.targetProfile
    ? `Profil: ${info.targetProfile}`
    : info.targetAgentId
      ? `Ajan: ${info.targetAgentId}`
      : '—'
  const tokens = `${formatTokens(info.inputTokens ?? 0)} giriş · ${formatTokens(info.outputTokens ?? 0)} çıkış`

  return (
    <Section title="Yürütme">
      <Row
        label="Tür / kategori"
        value={`${info.executionType ?? '—'} / ${info.category ?? '—'}`}
      />
      <Row label="Hedef" value={target} />
      <Row label="Bağlam" value={info.contextMode ?? '—'} />
      <Row
        label="Durum"
        value={
          terminalLabel[info.runState ?? ''] ?? info.runState ?? (info.terminal ? 'Terminal' : '—')
        }
      />
      <Row label="Süre" value={formatDurationMs(info.durationMs ?? 0)} />
      <Row label="Token" value={tokens} />
      <Row label="Araç çağrısı" value={String(info.toolCallCount ?? 0)} />
      <Row label="Kalıcı adım" value={String(info.persistedSteps ?? 0)} />
      {info.stopReason && <Row label="Stop reason" value={info.stopReason} />}
      {info.errorSummary && <Row label="Hata özeti" value={info.errorSummary} />}
    </Section>
  )
}
