import { RotateCcw } from 'lucide-react'
import type { AgentOverrideKey } from '@/types'
import { useTranslation } from 'react-i18next'

interface Props {
  field: AgentOverrideKey
  /** Whether the agent pins this field to its own value. */
  overridden: boolean
  /** Name of the agent the field would be inherited from. */
  parentName?: string
  /** Release the override so the field inherits again. */
  onReset: () => void
  disabled?: boolean
}

// FieldOverrideBadge sits at the right of a field label on a DERIVED agent and
// makes the inheritance state legible per field: a muted "devralındı" pill when
// the value comes from the parent, an accent "override" pill plus a one-click
// reset when the agent owns it. The pill is the only place this state is
// shown, so it must be unmistakable — hence colour AND wording, never colour
// alone.
export function FieldOverrideBadge({ field, overridden, parentName, onReset, disabled }: Props) {
  const { t } = useTranslation('agents')
  if (!overridden) {
    return (
      <span
        data-testid="agent-field-inherited"
        data-field={field}
        title={
          parentName
            ? t('inheritance.fromParent', { name: parentName })
            : t('inheritance.fromParentGeneric')
        }
        className="shrink-0 rounded bg-[var(--color-surface-2)] px-1.5 py-px text-[10px] font-normal text-[var(--color-text-dim)]"
      >
        {t('inheritance.inherited')}
        {parentName ? ` ← ${parentName}` : ''}
      </span>
    )
  }
  return (
    <span className="flex shrink-0 items-center gap-1">
      <span
        data-testid="agent-field-overridden"
        data-field={field}
        title={t('inheritance.overrideTitle')}
        className="rounded bg-[color-mix(in_srgb,var(--color-accent)_18%,transparent)] px-1.5 py-px text-[10px] font-medium text-[var(--color-accent)]"
      >
        {t('inheritance.override')}
      </span>
      <button
        type="button"
        data-testid="agent-field-reset"
        data-field={field}
        onClick={onReset}
        disabled={disabled}
        title={
          parentName ? t('inheritance.resetNamed', { name: parentName }) : t('inheritance.reset')
        }
        className="flex items-center gap-0.5 rounded px-1 py-px text-[10px] text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)] disabled:opacity-50"
      >
        <RotateCcw size={10} /> {t('inheritance.action')}
      </button>
    </span>
  )
}
