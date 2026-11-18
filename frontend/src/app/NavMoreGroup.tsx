import { ChevronDown, Ellipsis } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { MORE_NAV } from './navItems'
import type { View } from './viewMetadata'
import { NavItemButton, type NavSignals } from './NavItemButton'
import { navItemClass } from './navItemClass'
import { ActiveBar, NavDots } from './navRailParts'
import { rollupSignals } from './navSignals'

const GROUP_ID = 'nav-more-group'

interface Props extends NavSignals {
  view: View
  open: boolean
  onToggle: () => void
  onSelect: (view: View) => void
}

// NavMoreGroup is the labelled rail's disclosure for the less frequently used
// destinations. Closed, the toggle stands in for the group: it carries the
// active indicator and the rolled-up notification dots of the hidden items.
export function NavMoreGroup({ view, open, onToggle, onSelect, ...signals }: Props) {
  const { t } = useTranslation()
  const activeInside = MORE_NAV.some((item) => item.key === view)
  const showsActive = activeInside && !open
  const rollup = rollupSignals(MORE_NAV, signals)
  return (
    <>
      <button
        type="button"
        onClick={onToggle}
        data-testid="nav-more"
        aria-expanded={open}
        aria-controls={GROUP_ID}
        className={navItemClass(showsActive, false)}
      >
        {showsActive && <ActiveBar />}
        <Ellipsis size={18} strokeWidth={2} className="shrink-0" />
        <span>{t('shell.moreNavigation')}</span>
        <span className="ml-auto flex items-center gap-1.5">
          {!open && <NavDots {...rollup} />}
          <ChevronDown
            size={14}
            aria-hidden
            className={`shrink-0 transition-transform ${open ? 'rotate-180' : ''}`}
          />
        </span>
      </button>
      {open && (
        <div
          id={GROUP_ID}
          role="group"
          aria-label={t('shell.moreNavigation')}
          className="flex flex-col gap-1"
        >
          {MORE_NAV.map((item) => (
            <NavItemButton
              key={item.key}
              item={item}
              active={view === item.key}
              collapsed={false}
              onSelect={onSelect}
              {...signals}
            />
          ))}
        </div>
      )}
    </>
  )
}
