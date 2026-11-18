import { useTranslation } from 'react-i18next'

// ActiveBar is the accent indicator on the left edge of the active item.
export function ActiveBar() {
  return (
    <span className="absolute left-0 top-1/2 h-5 w-[3px] -translate-y-1/2 rounded-full bg-[var(--color-accent)]" />
  )
}

// NavDots renders the per-item notification cluster: an amber dot for unsaved
// edits (dirty) plus an accent dot for activity — pulsing when busy, solid when
// merely unread. Collapsed rail uses corner dots; expanded uses a right cluster.
export function NavDots({
  busy,
  unread,
  dirty,
  collapsed,
}: {
  busy?: boolean
  unread?: boolean
  dirty?: boolean
  collapsed?: boolean
}) {
  const { t } = useTranslation()
  if (!busy && !unread && !dirty) return null
  const accentTitle = busy ? t('shell.busy') : t('shell.newActivity')
  if (collapsed) {
    return (
      <>
        {dirty && (
          <span
            className="absolute left-1 top-1 h-2 w-2 rounded-full bg-[var(--color-warning)] ring-2 ring-[var(--color-surface)]"
            title={t('shell.unsaved')}
          />
        )}
        {(busy || unread) && (
          <span
            className={`absolute right-1 top-1 h-2 w-2 rounded-full bg-[var(--color-accent)] ring-2 ring-[var(--color-surface)] ${
              busy ? 'animate-pulse' : ''
            }`}
            title={accentTitle}
          />
        )}
      </>
    )
  }
  return (
    <span className="ml-auto flex items-center gap-1.5">
      {dirty && (
        <span
          className="h-2 w-2 rounded-full bg-[var(--color-warning)]"
          title={t('shell.unsaved')}
        />
      )}
      {(busy || unread) && (
        <span
          className={`h-2 w-2 rounded-full bg-[var(--color-accent)] ${busy ? 'animate-pulse' : ''}`}
          title={accentTitle}
        />
      )}
    </span>
  )
}
