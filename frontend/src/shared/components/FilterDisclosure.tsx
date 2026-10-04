import { ChevronRight, SlidersHorizontal } from 'lucide-react'
import { useTranslation } from 'react-i18next'

// FilterToggle folds a filter strip; useFilterDisclosure (./useFilterDisclosure)
// holds the remembered open state. A folded strip still badges how many filters
// are narrowing the list, so a hidden filter never silently hides rows.
interface ToggleProps {
  open: boolean
  onToggle: () => void
  // Visible label; defaults to the shared "Filtreler".
  label?: string
  // Filters currently narrowing the list — badged so a folded strip still says so.
  activeCount?: number
  className?: string
  testId?: string
}

// FilterToggle is the chevron button that folds a filter strip. Callers render
// the strip body themselves under `open`, so it fits both a stacked header and
// the first slot of an inline toolbar row.
export function FilterToggle({
  open,
  onToggle,
  label,
  activeCount = 0,
  className = '',
  testId,
}: ToggleProps) {
  const { t } = useTranslation('sharedUi')
  return (
    <button
      type="button"
      onClick={onToggle}
      aria-expanded={open}
      data-testid={testId}
      title={open ? t('filters.hide') : t('filters.show')}
      className={`group flex shrink-0 items-center gap-1 rounded text-[11px] text-[var(--color-text-dim)] transition hover:text-[var(--color-text)] ${className}`}
    >
      <ChevronRight size={12} className={`transition-transform ${open ? 'rotate-90' : ''}`} />
      <SlidersHorizontal size={11} className="opacity-70" />
      <span>{label ?? t('filters.label')}</span>
      {activeCount > 0 && (
        <span
          className="rounded-full bg-[var(--color-accent-soft)] px-1.5 text-[10px] font-medium text-[var(--color-accent)]"
          title={t('filters.active', { count: activeCount })}
        >
          {activeCount}
        </span>
      )}
    </button>
  )
}
