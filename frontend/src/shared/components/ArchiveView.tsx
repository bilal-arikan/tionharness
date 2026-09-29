import { Archive } from 'lucide-react'
import { useTranslation } from 'react-i18next'

// Shared "archive view" chrome, lifted from the kanban board so every list that
// can archive its items (tasks, agents, skills, automations) switches views the
// same way: one toggle in the header, one info bar above the archived list.

const TOGGLE_BASE =
  'flex flex-shrink-0 items-center gap-1 rounded border px-2 py-1 text-xs transition'
const TOGGLE_ON =
  'border-[var(--color-accent)] bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
const TOGGLE_OFF =
  'border-[var(--color-border)] text-[var(--color-text-dim)] hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]'

interface ToggleProps {
  /** True while the archive view is shown. */
  active: boolean
  onToggle: () => void
  testId?: string
  /** Label that leads back to the live list (e.g. "Panoya dön"). */
  backLabel?: string
  /** Tooltip while the live list is shown. */
  showTitle?: string
  /** Tooltip while the archive view is shown. */
  backTitle?: string
}

/** ArchiveViewToggle switches a list between its live items and its archive. */
export function ArchiveViewToggle({
  active,
  onToggle,
  testId,
  backLabel,
  showTitle,
  backTitle,
}: ToggleProps) {
  const { t } = useTranslation('sharedUi')
  const resolvedBackLabel = backLabel ?? t('archive.backToList')
  const resolvedShowTitle = showTitle ?? t('archive.show')
  const resolvedBackTitle = backTitle ?? t('archive.backToActive')
  return (
    <button
      type="button"
      data-testid={testId}
      aria-pressed={active}
      onClick={onToggle}
      title={active ? resolvedBackTitle : resolvedShowTitle}
      className={`${TOGGLE_BASE} ${active ? TOGGLE_ON : TOGGLE_OFF}`}
    >
      <Archive size={13} /> {active ? resolvedBackLabel : t('archive.label')}
    </button>
  )
}

interface BannerProps {
  /** How many archived items the view lists. */
  count: number
  /** Singular noun for the item ("görev", "ajan", "skill"). */
  noun: string
  /** How an item is restored, appended to the non-empty message. */
  restoreHint?: string
  testId?: string
}

/** ArchiveViewBanner is the info bar above an archive view. */
export function ArchiveViewBanner({ count, noun, restoreHint, testId }: BannerProps) {
  const { t } = useTranslation('sharedUi')
  return (
    <div
      data-testid={testId}
      className="flex items-center gap-2 border-b border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-1.5 text-xs text-[var(--color-text-dim)]"
    >
      <Archive size={13} className="flex-shrink-0" />
      <span className="min-w-0 flex-1">
        {count === 0
          ? t('archive.empty', { noun })
          : t('archive.count', {
              count,
              noun,
              restoreHint: restoreHint ? ` — ${restoreHint}` : '',
            })}
      </span>
    </div>
  )
}
