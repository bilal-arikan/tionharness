// The decision authorities section, grouped the way the server groups them
// (safety, coordination, flows …). Edits land in the settings draft and are
// saved with the page's Save button.
import { useTranslation } from 'react-i18next'
import type { DeciderConfig, DeciderView } from '@/types/decider'
import { SectionHead } from '@/shared/components'
import { defaultModelId, groupAuthorities, modelLabel, statsFor } from './deciderModel'
import { DeciderAuthorityRow } from './DeciderAuthorityRow'

interface Props {
  view: DeciderView
  draft: DeciderConfig
  onChange: (next: DeciderConfig) => void
}

export function DeciderAuthorities({ view, draft, onChange }: Props) {
  const { t } = useTranslation('decider')
  const defaultId = defaultModelId(draft, view.models)
  const defaultLabel = modelLabel(view.models, defaultId)
  return (
    <section
      className="space-y-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
      data-testid="decider-authorities"
    >
      <SectionHead>{t('authorities.title')}</SectionHead>
      <p className="text-xs text-[var(--color-text-dim)]">{t('authorities.hint')}</p>
      {groupAuthorities(view.authorities, view.groups).map(({ group, items }) => (
        <div key={group} className="space-y-2">
          <p className="text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
            {t(`group.${group}`, { defaultValue: group })}
          </p>
          {items.map((a) => (
            <DeciderAuthorityRow
              key={a.id}
              authority={a}
              draft={draft}
              models={view.models}
              defaultId={defaultId}
              defaultLabel={defaultLabel}
              stats={statsFor(view.stats, a.id)}
              onChange={onChange}
            />
          ))}
        </div>
      ))}
    </section>
  )
}
