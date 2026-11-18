import type { NavItem, NavSignals } from './NavItemButton'

// rollupSignals folds the per-view signals of a group of destinations into one
// set of flags, for the control that stands in for the whole group.
export function rollupSignals(items: readonly NavItem[], signals: NavSignals) {
  return {
    busy: items.some((item) => signals.busyViews?.has(item.key)),
    unread: items.some((item) => signals.unreadViews?.has(item.key)),
    dirty: items.some((item) => signals.dirtyViews?.has(item.key)),
  }
}
