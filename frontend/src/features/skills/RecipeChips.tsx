// RecipeChips — what a coordinator-workflow skill's structured plan (R6)
// looks like on the Skills screen: version, pattern, phase count with the
// phase ids, watchers / optimizer, and the parse error when the block is
// invalid (the recipe then still runs as prose but seeds no trajectory).
import type { Skill } from '@/types'

interface Props {
  skill: Skill
  // compact: list row (one chip); full: detail header (all chips).
  compact?: boolean
}

export function RecipeChips({ skill: sk, compact = false }: Props) {
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
            title={`Faz bloğu geçersiz: ${sk.recipeError}`}
          >
            ⚠ reçete
          </span>
        ) : (
          <span
            className={muted}
            title={
              phases.length
                ? `Fazlar: ${phases.map((p) => p.id).join(' → ')}`
                : 'Yapısal faz bloğu yok (düz yazı reçete)'
            }
          >
            reçete{phases.length ? ` · ${phases.length} faz` : ''}
          </span>
        )}
      </span>
    )
  }
  return (
    <>
      {sk.version && (
        <span
          className={muted}
          title="Reçete sürümü (frontmatter version); rota TemplateRef'inde slug@version olarak kaydedilir"
        >
          v{sk.version}
        </span>
      )}
      {sk.pattern && (
        <span className={muted} title="Orkestrasyon deseni">
          {sk.pattern}
        </span>
      )}
      {sk.recipeError ? (
        <span
          className={`${chip} bg-[color-mix(in_srgb,var(--color-danger)_16%,transparent)] text-[var(--color-danger)]`}
          title={sk.recipeError}
        >
          ⚠ faz bloğu geçersiz: {sk.recipeError.slice(0, 60)}
        </span>
      ) : phases.length > 0 ? (
        <span
          className={`${chip} bg-[var(--color-accent-soft)] text-[var(--color-accent)]`}
          title={phases
            .map(
              (p) =>
                `${p.id}${p.profile ? ` (${p.profile})` : ''}${p.gate ? ` · kapı ${p.gate.kind}` : ''}${p.optional ? ' · isteğe bağlı' : ''}`,
            )
            .join('\n')}
        >
          ◈ {phases.map((p) => p.label || p.id).join(' → ')}
        </span>
      ) : (
        <span
          className={muted}
          title="Bu reçete düz yazı: rota tohumlanmaz, koordinatör kendi planını trajectory{plan} ile ilan eder"
        >
          faz bloğu yok
        </span>
      )}
      {r?.watchers?.length ? (
        <span className={muted} title="Rota sonunda ateşlenecek izleyici otomasyonlar">
          ⚡ {r.watchers.join(', ')}
        </span>
      ) : null}
      {r?.optimizer && (
        <span className={muted} title="Rota sonunda koşacak optimizer">
          ✦ {r.optimizer}
        </span>
      )}
    </>
  )
}
