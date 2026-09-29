import { useTranslation } from 'react-i18next'

// Origin chip for an artifact (chat / manual / agent / tool / plan). Split out of
// artifactMeta so that module stays component-free (fast refresh).
const ORIGIN_META: Record<string, { cls: string }> = {
  chat: { cls: 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]' },
  manual: { cls: 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]' },
  agent: {
    cls: 'bg-[color-mix(in_srgb,var(--color-success)_18%,transparent)] text-[var(--color-success)]',
  },
  tool: {
    cls: 'bg-[color-mix(in_srgb,var(--color-warning)_18%,transparent)] text-[var(--color-warning)]',
  },
  plan: {
    cls: 'bg-[color-mix(in_srgb,var(--color-accent)_22%,transparent)] text-[var(--color-accent)]',
  },
}

export function OriginBadge({ origin }: { origin?: string }) {
  const { t } = useTranslation('artifacts')
  const meta = origin ? ORIGIN_META[origin] : undefined
  if (!meta) return null
  return (
    <span className={`rounded px-1.5 py-0.5 text-[10px] font-medium ${meta.cls}`}>
      {origin === 'plan' ? '📋 ' : ''}
      {t(`origin.${origin}`)}
    </span>
  )
}

// Manually creatable kinds (text-based). Media/file kinds arrive via drag-drop.
