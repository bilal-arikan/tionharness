// RecipeChips — what a coordinator-workflow skill's structured plan (R6)
// looks like on the Skills screen: version, pattern, phase count with the
// phase ids, watchers / optimizer, and the parse error when the block is
// invalid (the recipe then still runs as prose but seeds no trajectory).
import type { Skill } from '@/types'
import { useTranslation } from 'react-i18next'

interface Props {
  skill: Skill
  // compact: list row (one chip); full: detail header (all chips).
  compact?: boolean
}

export function RecipeChips({ skill: sk, compact = false }: Props) {
  const { t } = useTranslation('skills')
  if (sk.kind !== 'coordinator-workflow') return null
  const r = sk.recipe
  const phases = r?.phases ?? []
  const chip = 'rounded px-1.5 py-0.5 text-[10px] font-medium'
  const muted = `${chip} bg-[var(--color-surface-2)] text-[var(--color-text-dim)]`
  if (compact) {
    return (
      <span className="flex items-center gap-1">
        {sk.recipeError ? (
          <span
            className={`${chip} bg-[color-mix(in_srgb,var(--color-danger)_16%,transparent)] text-[var(--color-danger)]`}
            title={t('recipe.invalidBlock', { error: sk.recipeError })}
          >
            ⚠ {t('recipe.recipe')}
          </span>
        ) : (
          <span
            className={muted}
            title={
              phases.length
                ? t('recipe.phases', { phases: phases.map((p) => p.id).join(' → ') })
                : t('recipe.noStructuredPhases')
            }
          >
            {t('recipe.recipe')}
            {phases.length ? ` · ${t('recipe.phaseCount', { count: phases.length })}` : ''}
          </span>
        )}
      </span>
    )
  }
  return (
    <>
      {sk.version && (
        <span className={muted} title={t('recipe.versionHint')}>
          {t('recipe.version', { version: sk.version })}
        </span>
      )}
      {sk.pattern && (
        <span className={muted} title={t('recipe.patternHint')}>
          {sk.pattern}
        </span>
      )}
      {sk.recipeError ? (
        <span
          className={`${chip} bg-[color-mix(in_srgb,var(--color-danger)_16%,transparent)] text-[var(--color-danger)]`}
          title={sk.recipeError}
        >
          ⚠ {t('recipe.invalidBlockShort', { error: sk.recipeError.slice(0, 60) })}
        </span>
      ) : phases.length > 0 ? (
        <span
          className={`${chip} bg-[var(--color-accent-soft)] text-[var(--color-accent)]`}
          title={phases
            .map(
              (p) =>
                `${p.id}${p.profile ? ` (${p.profile})` : ''}${p.gate ? ` · ${t('recipe.gate')} ${p.gate.kind}` : ''}${p.optional ? ` · ${t('recipe.optional')}` : ''}`,
            )
            .join('\n')}
        >
          {t('recipe.phasePlan', { phases: phases.map((p) => p.label || p.id).join(' → ') })}
        </span>
      ) : (
        <span className={muted} title={t('recipe.proseHint')}>
          {t('recipe.noPhaseBlock')}
        </span>
      )}
      {r?.watchers?.length ? (
        <span className={muted} title={t('recipe.watchersHint')}>
          ⚡ {r.watchers.join(', ')}
        </span>
      ) : null}
      {r?.optimizer && (
        <span className={muted} title={t('recipe.optimizerHint')}>
          {t('recipe.optimizer', { optimizer: r.optimizer })}
        </span>
      )}
      {r?.autoPrune && (
        <span
          className={`${chip} bg-[color-mix(in_srgb,var(--color-warning)_16%,transparent)] text-[var(--color-warning)]`}
          title={t('recipe.autoPruneHint')}
        >
          ✂ {t('recipe.autoPrune')}
        </span>
      )}
    </>
  )
}
