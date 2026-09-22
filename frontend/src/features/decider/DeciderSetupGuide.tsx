// The three-step setup of the decision layer, shown on both screens until every
// step is done: a credential (a provider account to borrow, or a decision
// provider's own key), a ready decision provider, the master switch. Each step
// carries the action that finishes it on the current screen, or a link to the
// screen that does.
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Check } from 'lucide-react'
import type { SetupStep, SetupStepId } from './deciderModel'
import { cardCls } from './providerStyles'

interface Props {
  steps: SetupStep[]
  // actions maps a step to the control that finishes it here (a button, a
  // link to the other screen); a step without one shows only its text.
  actions?: Partial<Record<SetupStepId, ReactNode>>
}

export function DeciderSetupGuide({ steps, actions = {} }: Props) {
  const { t } = useTranslation('decider')
  if (steps.every((s) => s.done)) return null
  return (
    <div className={cardCls} data-testid="decider-setup">
      <p className="text-sm font-medium">{t('setup.title')}</p>
      <ol className="space-y-2">
        {steps.map((s, i) => (
          <li key={s.id} className="flex gap-2 text-xs" data-testid={`decider-setup-${s.id}`}>
            <span
              className={`flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-[10px] font-semibold ${
                s.done
                  ? 'bg-[color-mix(in_srgb,var(--color-success)_18%,transparent)] text-[var(--color-success)]'
                  : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
              }`}
              aria-label={s.done ? t('setup.done') : undefined}
            >
              {s.done ? <Check size={12} /> : i + 1}
            </span>
            <div className="min-w-0 flex-1 space-y-1">
              <p
                className={
                  s.done ? 'text-[var(--color-text-dim)] line-through' : 'text-[var(--color-text)]'
                }
              >
                <span className="font-medium">{t(`setup.${s.id}.title`)}</span>
                {' — '}
                {t(`setup.${s.id}.text`)}
              </p>
              {!s.done && actions[s.id]}
            </div>
          </li>
        ))}
      </ol>
    </div>
  )
}
