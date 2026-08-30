/* eslint-disable react-hooks/refs -- `drag` is the object returned by useDragScroll;
   reading drag.ref / drag.onMouseDown to spread onto JSX is a plain property read,
   not a ref *dereference* during render. The rule matches on the `.ref` name. */
import { Boxes, Settings, type LucideIcon } from 'lucide-react'
import { NAV } from './navItems'
import type { View } from './NavRail'
import { useDragScroll } from '@/shared/hooks/useDragScroll'
import { MobileWorkspaceButton } from './MobileWorkspaceButton'
import type { Workspace } from '@/types'
import type { NewWorkspaceData } from '@/features/workspace/WorkspaceCreateModal'
import { useTranslation } from 'react-i18next'

interface Props {
  view: View
  onSelectView: (v: View) => void
  busyViews?: Set<View>
  unreadViews?: Set<View>
  dirtyViews?: Set<View>
  // Workspace picker (pinned at the strip's start) — the mobile counterpart of
  // the desktop rail's WorkspaceSwitcher.
  workspaces: Workspace[]
  activeWorkspaceId: string | null
  unreadWorkspaceIds?: Set<string>
  // Workspaces with a live run — pulses a "çalışıyor" dot in the mobile picker.
  busyWorkspaceIds?: Set<string>
  onSwitchWorkspace: (id: string) => void
  onCreateWorkspace: (data: NewWorkspaceData) => void
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
export function MobileNavBar({
  view,
  onSelectView,
  busyViews,
  unreadViews,
  dirtyViews,
  workspaces,
  activeWorkspaceId,
  unreadWorkspaceIds,
  busyWorkspaceIds,
  onSwitchWorkspace,
  onCreateWorkspace,
}: Props) {
  const { t } = useTranslation()
  const items = [...NAV, ...PINNED]
  // Mouse click-and-drag panning (touch already scrolls natively).
  const drag = useDragScroll<HTMLDivElement>()
  return (
    // The nav itself does NOT scroll (overflow visible) so the workspace picker's
    // upward popup is not clipped; only the inner view strip scrolls horizontally.
    <nav
      aria-label="Ana gezinme"
      className="fixed inset-x-0 bottom-0 z-50 flex items-stretch border-t border-[var(--color-border)] bg-[var(--color-surface)] pt-1 shadow-[var(--shadow-sm)] md:hidden"
      style={{ paddingBottom: 'max(0.25rem, env(safe-area-inset-bottom))' }}
    >
      {/* Pinned workspace picker at the very start (always reachable). */}
      <MobileWorkspaceButton
        workspaces={workspaces}
        activeId={activeWorkspaceId}
        unreadIds={unreadWorkspaceIds}
        busyIds={busyWorkspaceIds}
        onSwitch={onSwitchWorkspace}
        onCreate={onCreateWorkspace}
      />

      {/* Horizontally-scrollable view strip (mouse drag + native touch). */}
      <div
        ref={drag.ref}
        onMouseDown={drag.onMouseDown}
        onClickCapture={drag.onClickCapture}
        className="flex flex-1 cursor-grab gap-1 overflow-x-auto px-2 select-none active:cursor-grabbing [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
      >
        {items.map((item) => {
          const label = item.key === 'prompts' ? t('navigation.promptsFiles') : item.label
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
              aria-label={label}
              aria-current={active ? 'page' : undefined}
              className={`relative flex min-w-[3.75rem] shrink-0 flex-col items-center gap-0.5 rounded-lg px-2 py-1.5 text-[10px] leading-none transition ${
                active
                  ? 'bg-[var(--color-accent-soft)] font-medium text-[var(--color-accent)]'
                  : 'text-[var(--color-text-dim)]'
              }`}
            >
              <Icon size={18} strokeWidth={2} className="shrink-0" />
              <span className="max-w-[4.5rem] truncate">{label}</span>
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
      </div>
    </nav>
  )
}
