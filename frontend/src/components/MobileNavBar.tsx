import { Boxes, Settings, type LucideIcon } from 'lucide-react'
import { NAV, type View } from './NavRail'

interface Props {
  view: View
  onSelectView: (v: View) => void
  busyViews?: Set<View>
  unreadViews?: Set<View>
  dirtyViews?: Set<View>
}

// The two pinned items that sit below the primary NAV list in the desktop rail.
// Appended after NAV so the mobile bar exposes the exact same destinations.
const PINNED: { key: View; label: string; icon: LucideIcon }[] = [
  { key: 'workspace', label: 'Workspace', icon: Boxes },
  { key: 'settings', label: 'Ayarlar', icon: Settings },
]

// MobileNavBar is the bottom navigation for portrait phones (`< md`). The desktop
// vertical NavRail is hidden at this breakpoint; here every view lives in a single
// horizontally-scrollable strip so all destinations stay reachable with a swipe —
// no "more" drawer, no hidden items. Hidden on `md+` where the rail takes over.
export function MobileNavBar({ view, onSelectView, busyViews, unreadViews, dirtyViews }: Props) {
  const items = [...NAV, ...PINNED]
  return (
    <nav
      aria-label="Ana gezinme"
      className="fixed inset-x-0 bottom-0 z-50 flex gap-1 overflow-x-auto border-t border-[var(--color-border)] bg-[var(--color-surface)] px-2 pt-1 shadow-[var(--shadow-sm)] md:hidden [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
      style={{ paddingBottom: 'max(0.25rem, env(safe-area-inset-bottom))' }}
    >
      {items.map((item) => {
        const Icon = item.icon
        const active = view === item.key
        const busy = busyViews?.has(item.key) ?? false
        const unread = unreadViews?.has(item.key) ?? false
        const dirty = dirtyViews?.has(item.key) ?? false
        return (
          <button
            key={item.key}
            onClick={() => onSelectView(item.key)}
            data-testid={`mnav-${item.key}`}
            aria-label={item.label}
            aria-current={active ? 'page' : undefined}
            className={`relative flex min-w-[3.75rem] shrink-0 flex-col items-center gap-0.5 rounded-lg px-2 py-1.5 text-[10px] leading-none transition ${
              active
                ? 'bg-[var(--color-accent-soft)] font-medium text-[var(--color-accent)]'
                : 'text-[var(--color-text-dim)]'
            }`}
          >
            <Icon size={18} strokeWidth={2} className="shrink-0" />
            <span className="max-w-[4.5rem] truncate">{item.label}</span>
            {dirty && (
              <span
                className="absolute left-2 top-1 h-1.5 w-1.5 rounded-full bg-[var(--color-warning)]"
                title="Kaydedilmemiş değişiklik"
              />
            )}
            {(busy || unread) && (
              <span
                className={`absolute right-2 top-1 h-1.5 w-1.5 rounded-full bg-[var(--color-accent)] ${
                  busy ? 'animate-pulse' : ''
                }`}
                title={busy ? 'İşlem sürüyor' : 'Yeni etkinlik'}
              />
            )}
          </button>
        )
      })}
    </nav>
  )
}
