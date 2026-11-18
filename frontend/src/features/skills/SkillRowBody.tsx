import { useTranslation } from 'react-i18next'
import type { Skill } from '@/types'
import { SeedDefaultBadge } from '@/shared/components'
import { fullDateTime, relativeTime } from '@/shared/lib/time'
import { RestrictedBadge, SourceBadge, VisibilityChip } from './SkillBadges'
import { skillVisibility } from './skillVisibility'

// SkillRowBody is the content of one row in the skills list.
//
// The name owns the whole first line: the badges used to share it and left the
// name about eight characters ("Günlük Ha…"). They now sit on a meta line under
// the description, in the quiet badge style, next to the last-modified time.
export function SkillRowBody({ skill: sk }: { skill: Skill }) {
  const { t } = useTranslation('skills')
  return (
    <>
      <span className="mt-0.5 shrink-0 text-base leading-none">{sk.icon || '✨'}</span>
      <span className="min-w-0 flex-1">
        <span data-testid="skill-row-name" className="block truncate font-medium" title={sk.name}>
          {sk.name}
        </span>
        <span className="mt-0.5 block truncate text-[11px] text-[var(--color-text-dim)]">
          {sk.description || sk.slug}
        </span>
        <span data-testid="skill-row-meta" className="mt-1 flex flex-wrap items-center gap-1.5">
          {!sk.shared && <RestrictedBadge quiet />}
          <VisibilityChip v={skillVisibility(sk)} quiet />
          <SourceBadge source={sk.source} quiet />
          <SeedDefaultBadge state={sk.defaultState} />
          {sk.modifiedAt ? (
            <span
              className="text-[11px] text-[var(--color-text-dim)] opacity-70"
              title={t('panel.lastModifiedFull', { date: fullDateTime(sk.modifiedAt) })}
            >
              {t('panel.modified')}: {relativeTime(sk.modifiedAt)}
            </span>
          ) : null}
        </span>
      </span>
    </>
  )
}
