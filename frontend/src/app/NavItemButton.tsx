import { useTranslation } from 'react-i18next'
import type { NAV } from './navItems'
import type { View } from './viewMetadata'
import { navItemClass } from './navItemClass'
import { ActiveBar, NavDots } from './navRailParts'

export type NavItem = (typeof NAV)[number]

// Per-view notification sets (scoped to the active workspace).
export interface NavSignals {
  busyViews?: Set<View>
  unreadViews?: Set<View>
  dirtyViews?: Set<View>
}

interface Props extends NavSignals {
  item: NavItem
  active: boolean
  collapsed: boolean
  onSelect: (view: View) => void
}

// NavItemButton is one destination in the desktop rail: icon, label (hidden in
// the icon-only rail, where it moves to the tooltip) and the notification dots.
export function NavItemButton({
  item,
  active,
  collapsed,
  busyViews,
  unreadViews,
  dirtyViews,
  onSelect,
}: Props) {
  const { t } = useTranslation()
  const label = t(item.labelKey)
  const Icon = item.icon
  const busy = busyViews?.has(item.key) ?? false
  const unread = unreadViews?.has(item.key) ?? false
  const dirty = dirtyViews?.has(item.key) ?? false
  return (
    <button
      onClick={() => onSelect(item.key)}
      data-testid={`nav-${item.key}`}
      aria-label={label}
      aria-current={active ? 'page' : undefined}
      title={
        collapsed
          ? `${label}${busy ? ` · ${t('shell.busy')}` : unread ? ` · ${t('shell.newActivity')}` : ''}`
          : undefined
      }
      className={navItemClass(active, collapsed)}
    >
      {active && <ActiveBar />}
      <Icon size={18} strokeWidth={2} className="shrink-0" />
      {!collapsed && <span>{label}</span>}
      <NavDots busy={busy} unread={unread} dirty={dirty} collapsed={collapsed} />
    </button>
  )
}
