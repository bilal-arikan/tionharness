// Backdrop is the dim scrim behind every slide-in drawer (sessions list, detail
// panel, list panes, the peeking nav rail). One component so the colour, z-index
// and the fade-in transition (styles/layout.css `.th-backdrop`) stay identical.
export function Backdrop({
  onClick,
  className = '',
  label,
}: {
  onClick: () => void
  className?: string
  label?: string
}) {
  const { t } = useTranslation('sharedUi')
  return (
    <div
      role="presentation"
      aria-label={label ?? t('actions.close')}
      onClick={onClick}
      className={`th-backdrop fixed inset-0 z-30 bg-[var(--color-overlay)]/50 ${className}`}
    />
  )
}
import { useTranslation } from 'react-i18next'
