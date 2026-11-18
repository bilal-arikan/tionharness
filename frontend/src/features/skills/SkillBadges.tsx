import { useTranslation } from 'react-i18next'
import type { SkillSource, ToolVisibility } from '@/types'
import { visibilityMeta } from '@/features/tools/toolMeta'

// Two looks for the skill badges:
//  - the detail header keeps the coloured, uppercase chips (one skill on screen,
//    so the colour has room to mean something);
//  - the list uses QUIET for all of them: with three coloured chips on every row
//    the badges outshout the names, so there they are one neutral, sentence-case
//    style and the label alone carries the meaning.
const LOUD = 'rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide'
const QUIET =
  'shrink-0 rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[11px] leading-none text-[var(--color-text-dim)]'

export function SourceBadge({ source, quiet }: { source: SkillSource; quiet?: boolean }) {
  const { t } = useTranslation('skills')
  if (quiet) return <span className={QUIET}>{t(`source.${source}`)}</span>
  // Complete class sets per source (see the note on QUIET/LOUD above).
  return (
    <span
      className={
        source === 'workspace'
          ? `${LOUD} bg-[var(--color-accent-soft)] text-[var(--color-accent)]`
          : `${LOUD} bg-[var(--color-surface-2)] text-[var(--color-text-dim)]`
      }
    >
      {t(`source.${source}`)}
    </span>
  )
}

// RestrictedBadge marks a skill that is NOT on-demand: only agents it is
// explicitly assigned to can see/use it. On-demand (shared) skills are the
// default case and carry no badge.
export function RestrictedBadge({ quiet }: { quiet?: boolean }) {
  const { t } = useTranslation('skills')
  return (
    <span
      className={
        quiet
          ? QUIET
          : `${LOUD} bg-[color-mix(in_srgb,var(--color-danger)_18%,transparent)] text-[var(--color-danger)]`
      }
      title={t('access.restrictedHint')}
    >
      {t('access.restricted')}
    </span>
  )
}

// VisibilityChip shows which of the four catalog-visibility tiers a skill is on
// (Tam / Özet / İsim / Gizli) as a single chip, sharing VISIBILITY_TIERS
// metadata with the tier selector, so every tier — including full/summary —
// reads at a glance.
export function VisibilityChip({ v, quiet }: { v: ToolVisibility; quiet?: boolean }) {
  const meta = visibilityMeta(v)
  if (quiet) {
    return (
      <span className={QUIET} title={meta.hint}>
        {meta.label}
      </span>
    )
  }
  return (
    <span
      className={LOUD}
      style={{
        backgroundColor: `color-mix(in srgb, ${meta.color} 18%, transparent)`,
        color: meta.color,
      }}
      title={meta.hint}
    >
      {meta.label}
    </span>
  )
}
