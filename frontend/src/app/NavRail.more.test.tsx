// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, expect, it, vi } from 'vitest'
import { NavRail, type View } from './NavRail'
import { MAIN_NAV, MORE_NAV } from './navItems'
import { i18next } from '@/i18n'

vi.mock('@/api', () => ({ api: {} }))
;(
  globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
).IS_REACT_ACT_ENVIRONMENT = true

// Wide viewport → the labelled rail; the icon-only rail is forced via the
// persisted collapse flag where a test needs it.
Object.defineProperty(window, 'innerWidth', { value: 1440, configurable: true })
Object.defineProperty(window, 'innerHeight', { value: 900, configurable: true })

const roots: ReturnType<typeof createRoot>[] = []

function renderRail(view: View, onSelectView: (v: View) => void = () => {}) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  const rail = (v: View) => (
    <NavRail
      view={v}
      onSelectView={onSelectView}
      workspaces={[]}
      activeWorkspaceId={null}
      unreadWorkspaceIds={new Set()}
      unreadViews={new Set<View>(['notes'])}
      onSwitchWorkspace={() => {}}
      onCreateWorkspace={() => {}}
    />
  )
  act(() => root.render(rail(view)))
  return { container, rerender: (v: View) => act(() => root.render(rail(v))) }
}

const byId = (root: ParentNode, id: string) =>
  root.querySelector<HTMLElement>(`[data-testid="${id}"]`)

afterEach(async () => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
  localStorage.clear()
  await i18next.changeLanguage('tr')
})

it('keeps the frequent destinations visible and the rest behind a persisted disclosure', async () => {
  await act(() => i18next.changeLanguage('tr'))
  const { container } = renderRail('dashboard')
  const more = byId(container, 'nav-more')!

  for (const item of MAIN_NAV) expect(byId(container, `nav-${item.key}`)).not.toBeNull()
  for (const item of MORE_NAV) expect(byId(container, `nav-${item.key}`)).toBeNull()
  expect(more.textContent).toContain('Daha fazla')
  expect(more.getAttribute('aria-expanded')).toBe('false')
  // The closed toggle carries the hidden items' notification dot.
  expect(more.querySelector('[title="Yeni etkinlik"]')).not.toBeNull()

  act(() => more.click())
  expect(more.getAttribute('aria-expanded')).toBe('true')
  expect(more.getAttribute('aria-controls')).toBe(byId(container, 'nav-notes')?.parentElement?.id)
  for (const item of MORE_NAV) expect(byId(container, `nav-${item.key}`)).not.toBeNull()
  expect(localStorage.getItem('tionharness.navMoreOpen')).toBe('1')

  act(() => more.click())
  expect(byId(container, 'nav-notes')).toBeNull()
  expect(localStorage.getItem('tionharness.navMoreOpen')).toBe('0')
})

it('opens the group by itself when the active view moves into it', () => {
  const { container, rerender } = renderRail('chat')
  expect(byId(container, 'nav-budget')).toBeNull()

  rerender('budget')
  expect(byId(container, 'nav-budget')?.getAttribute('aria-current')).toBe('page')

  // Collapsing it again while inside moves the active indicator to the toggle.
  const more = byId(container, 'nav-more')!
  act(() => more.click())
  expect(byId(container, 'nav-budget')).toBeNull()
  expect(more.className).toContain('text-[var(--color-accent)]')
})

it('starts open when the app boots on a view inside the group', () => {
  const { container } = renderRail('insights')
  expect(byId(container, 'nav-more')?.getAttribute('aria-expanded')).toBe('true')
  expect(byId(container, 'nav-insights')).not.toBeNull()
})

it('offers the group as a labelled keyboard menu in the icon-only rail', async () => {
  await act(() => i18next.changeLanguage('en'))
  localStorage.setItem('tionharness.navCollapsed', '1')
  const onSelect = vi.fn()
  const { container } = renderRail('dashboard', onSelect)
  const more = byId(container, 'nav-more')!

  expect(byId(container, 'nav-rail')?.getAttribute('data-rail-mode')).toBe('collapsed')
  expect(more.getAttribute('aria-haspopup')).toBe('menu')
  expect(more.getAttribute('aria-label')).toBe('More')
  expect(document.querySelector('[role="menu"]')).toBeNull()

  act(() => more.click())
  const menu = document.querySelector<HTMLElement>('[role="menu"]')!
  expect(more.getAttribute('aria-expanded')).toBe('true')
  const items = [...menu.querySelectorAll<HTMLElement>('[role="menuitem"]')]
  expect(items.map((el) => el.textContent)).toEqual([
    'Skills',
    'Tools',
    'Marketplace',
    'Budget',
    'Prompts',
    'Insights',
    'Notes',
  ])
  expect(document.activeElement).toBe(items[0])

  const key = (k: string) =>
    act(() => {
      document.activeElement?.dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true }))
    })
  key('ArrowDown')
  expect(document.activeElement).toBe(items[1])
  key('End')
  expect(document.activeElement).toBe(items[6])
  key('Escape')
  expect(document.querySelector('[role="menu"]')).toBeNull()
  expect(document.activeElement).toBe(more)

  act(() => more.click())
  act(() => byId(document, 'nav-market')?.click())
  expect(onSelect).toHaveBeenCalledWith('market')
  expect(document.querySelector('[role="menu"]')).toBeNull()
})
