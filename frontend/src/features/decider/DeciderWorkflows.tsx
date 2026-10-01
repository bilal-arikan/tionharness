import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { DeciderConfig, DeciderMode, DeciderView } from '@/types/decider'
import { Badge } from '@/shared/components'
import {
  authorityConfig,
  defaultModelId,
  groupAuthorities,
  modelLabel,
  statsFor,
} from './deciderModel'
import { DeciderAuthorityRow } from './DeciderAuthorityRow'

interface Props {
  view: DeciderView
  draft: DeciderConfig
  onChange: (next: DeciderConfig) => void
}

const WORKFLOW_GROUPS = ['session', 'context', 'collaboration']
const GROUP_LABELS: Record<string, string> = {
  session: 'Session start',
  context: 'Context',
  collaboration: 'Worker collaboration',
}
const MODE_TONE = { off: 'muted', shadow: 'warning', on: 'success' } as const satisfies Record<
  DeciderMode,
  string
>

export function DeciderWorkflows({ view, draft, onChange }: Props) {
  const { t } = useTranslation('decider')
  const [selectedId, setSelectedId] = useState<string>()
  const detailId = useId()
  const groups = groupAuthorities(view.authorities, [...WORKFLOW_GROUPS, ...view.groups])
  // A registry refresh can remove the selected authority. Keep the detail
  // panel usable without adding synthetic workflows to the registry.
  const selected =
    view.authorities.find((authority) => authority.id === selectedId) ?? groups[0]?.items[0]
  const defaultId = defaultModelId(draft, view.models)

  if (!selected) {
    return (
      <p className="text-sm text-[var(--color-text-dim)]" data-testid="decider-workflows-empty">
        {t('workflow.empty', { defaultValue: 'No decision workflows are available.' })}
      </p>
    )
  }

  return (
    <div className="min-w-0 space-y-3" data-testid="decider-workflows">
      {!draft.enabled && (
        <p
          className="rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] p-3 text-xs text-[var(--color-text-dim)]"
          data-testid="decider-workflows-disabled"
        >
          {t('workflow.masterDisabled', {
            defaultValue: 'Decision workflows are paused while the master switch is off.',
          })}
        </p>
      )}
      <div className="grid min-w-0 gap-4 md:grid-cols-[minmax(12rem,0.8fr)_minmax(0,2fr)]">
        <nav
          className="min-w-0 space-y-4"
          aria-label={t('workflow.selectHint', { defaultValue: 'Choose a decision workflow' })}
        >
          {groups.map(({ group, items }) => (
            <div key={group} className="min-w-0 space-y-1.5">
              <p className="text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
                {WORKFLOW_GROUPS.includes(group)
                  ? t(`workflow.groups.${group}`, { defaultValue: GROUP_LABELS[group] })
                  : t(`group.${group}`, { defaultValue: group })}
              </p>
              {items.map((authority) => {
                const active = authority.id === selected.id
                const mode = authorityConfig(draft, authority).mode
                return (
                  <button
                    key={authority.id}
                    type="button"
                    aria-pressed={active}
                    aria-controls={detailId}
                    onClick={() => setSelectedId(authority.id)}
                    data-testid={`decider-workflow-select-${authority.id}`}
                    className={`block min-h-11 w-full min-w-0 rounded-lg border p-3 text-start transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--color-accent)] ${
                      active
                        ? 'border-[var(--color-accent)] bg-[var(--color-bg)]'
                        : 'border-[var(--color-border)] hover:bg-[var(--color-bg)]'
                    }`}
                  >
                    <span className="flex min-w-0 flex-wrap items-center justify-between gap-2">
                      <span className="min-w-0 break-words text-sm font-medium">
                        {t(`authority.${authority.id}.label`, { defaultValue: authority.label })}
                      </span>
                      <Badge tone={MODE_TONE[mode]}>{t(`modeLabel.${mode}`)}</Badge>
                    </span>
                    <span className="mt-1 block break-words text-xs text-[var(--color-text-dim)]">
                      {t(`authority.${authority.id}.description`, {
                        defaultValue: authority.description,
                      })}
                    </span>
                  </button>
                )
              })}
            </div>
          ))}
        </nav>
        <div id={detailId} className="min-w-0 space-y-2" data-testid="decider-workflow-detail">
          <p className="text-xs text-[var(--color-text-dim)]">
            {t('workflow.detailHint', {
              defaultValue: 'Configure the selected workflow. Save applies your changes.',
            })}
          </p>
          <DeciderAuthorityRow
            key={selected.id}
            authority={selected}
            draft={draft}
            models={view.models}
            defaultId={defaultId}
            defaultLabel={modelLabel(view.models, defaultId)}
            stats={statsFor(view.stats, selected.id)}
            onChange={onChange}
          />
        </div>
      </div>
    </div>
  )
}
