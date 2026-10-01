// Workflow settings share the page draft and its Save action.
import { useTranslation } from 'react-i18next'
import type { DeciderConfig, DeciderView } from '@/types/decider'
import { SectionHead } from '@/shared/components'
import { DeciderWorkflows } from './DeciderWorkflows'

interface Props {
  view: DeciderView
  draft: DeciderConfig
  onChange: (next: DeciderConfig) => void
}

export function DeciderAuthorities({ view, draft, onChange }: Props) {
  const { t } = useTranslation('decider')
  return (
    <section
      className="space-y-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
      data-testid="decider-authorities"
    >
      <SectionHead>{t('authorities.title')}</SectionHead>
      <p className="text-xs text-[var(--color-text-dim)]">{t('authorities.hint')}</p>
      <DeciderWorkflows view={view} draft={draft} onChange={onChange} />
    </section>
  )
}
