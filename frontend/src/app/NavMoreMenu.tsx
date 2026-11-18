import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Ellipsis } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { MORE_NAV } from './navItems'
import type { View } from './viewMetadata'
import type { NavSignals } from './NavItemButton'
import { navItemClass } from './navItemClass'
import { ActiveBar, NavDots } from './navRailParts'
import { rollupSignals } from './navSignals'

const MENU_ID = 'nav-more-menu'
// Gap between the rail button and the menu, and the menu's viewport margin.
const GAP_PX = 8

interface Props extends NavSignals {
  view: View
  onSelect: (view: View) => void
}

// NavMoreMenu is the icon-only rail's counterpart of NavMoreGroup: one "more"
// button that opens a labelled popover menu beside the rail. The menu is
// portalled to <body> because the rail's list scrolls (and would clip it), and
// it is anchored by its bottom edge so it grows upward and stays on screen.
export function NavMoreMenu({ view, onSelect, ...signals }: Props) {
  const { t } = useTranslation()
  const [anchor, setAnchor] = useState<{ left: number; bottom: number } | null>(null)
  const buttonRef = useRef<HTMLButtonElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const open = anchor !== null
  const activeInside = MORE_NAV.some((item) => item.key === view)
  const rollup = rollupSignals(MORE_NAV, signals)
  const label = t('shell.moreNavigation')

  const close = (restoreFocus = false) => {
    setAnchor(null)
    if (restoreFocus) buttonRef.current?.focus()
  }
  const toggle = () => {
    if (open) return close()
    const rect = buttonRef.current?.getBoundingClientRect()
    if (!rect) return
    setAnchor({
      left: rect.right + GAP_PX,
      bottom: Math.max(GAP_PX, window.innerHeight - rect.bottom),
    })
  }

  // While open: move focus into the menu (the active destination if it is in
  // here, else the first), and close on an outside press or a viewport change
  // that would leave the menu detached from its button.
  useEffect(() => {
    if (!open) return
    const items = menuRef.current?.querySelectorAll<HTMLElement>('[role="menuitem"]')
    const current = menuRef.current?.querySelector<HTMLElement>('[aria-current="page"]')
    ;(current ?? items?.[0])?.focus()
    const onDown = (e: MouseEvent) => {
      const target = e.target as Node
      if (menuRef.current?.contains(target) || buttonRef.current?.contains(target)) return
      setAnchor(null)
    }
    const onResize = () => setAnchor(null)
    document.addEventListener('mousedown', onDown)
    window.addEventListener('resize', onResize)
    return () => {
      document.removeEventListener('mousedown', onDown)
      window.removeEventListener('resize', onResize)
    }
  }, [open])

  // Menu keyboard model: arrows / Home / End move between items, Escape closes
  // and returns focus to the button, Tab leaves (and closes) the menu.
  const onMenuKeyDown = (e: React.KeyboardEvent) => {
    const items = Array.from(
      menuRef.current?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [],
    )
    const index = items.indexOf(document.activeElement as HTMLElement)
    const focusAt = (i: number) => {
      e.preventDefault()
      items[(i + items.length) % items.length]?.focus()
    }
    if (e.key === 'ArrowDown') focusAt(index + 1)
    else if (e.key === 'ArrowUp') focusAt(index - 1)
    else if (e.key === 'Home') focusAt(0)
    else if (e.key === 'End') focusAt(items.length - 1)
    else if (e.key === 'Escape') {
      e.preventDefault()
      close(true)
    } else if (e.key === 'Tab') close()
  }

  return (
    <>
      <button
        ref={buttonRef}
        type="button"
        onClick={toggle}
        data-testid="nav-more"
        aria-label={label}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? MENU_ID : undefined}
        title={open ? undefined : label}
        className={navItemClass(activeInside, true)}
      >
        {activeInside && <ActiveBar />}
        <Ellipsis size={18} strokeWidth={2} className="shrink-0" />
        <NavDots {...rollup} collapsed />
      </button>
      {anchor &&
        createPortal(
          <div
            ref={menuRef}
            id={MENU_ID}
            role="menu"
            aria-label={label}
            data-testid="nav-more-menu"
            onKeyDown={onMenuKeyDown}
            style={{ left: anchor.left, bottom: anchor.bottom }}
            className="fixed z-50 flex max-h-[calc(100vh-1rem)] min-w-44 flex-col gap-0.5 overflow-y-auto rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-[var(--shadow-lg)]"
          >
            {MORE_NAV.map((item) => {
              const Icon = item.icon
              const isActive = view === item.key
              return (
                <button
                  key={item.key}
                  type="button"
                  role="menuitem"
                  data-testid={`nav-${item.key}`}
                  aria-current={isActive ? 'page' : undefined}
                  onClick={() => {
                    close()
                    onSelect(item.key)
                  }}
                  className={`${navItemClass(isActive, false)} text-left`}
                >
                  <Icon size={18} strokeWidth={2} className="shrink-0" />
                  <span>{t(item.labelKey)}</span>
                  <NavDots
                    busy={signals.busyViews?.has(item.key)}
                    unread={signals.unreadViews?.has(item.key)}
                    dirty={signals.dirtyViews?.has(item.key)}
                  />
                </button>
              )
            })}
          </div>,
          document.body,
        )}
    </>
  )
}
