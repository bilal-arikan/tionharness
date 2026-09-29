// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AppSettings } from '@/types'
import { api } from '@/api'
import { toast } from '@/shared/components'
import { i18next } from '@/i18n'
import { SettingsPanel } from './SettingsPanel'

vi.mock('@/api', () => ({
  api: { getSettings: vi.fn(), updateSettings: vi.fn(), getPrompts: vi.fn() },
}))
vi.mock('@/shared/components', () => ({
  Button: (props: React.ButtonHTMLAttributes<HTMLButtonElement>) => <button {...props} />,
  LoadingState: () => <p>Loading</p>,
  CollapsibleListShell: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  toast: { success: vi.fn() },
}))
vi.mock('@/shared/lib/dirtySignals', () => ({ useRegisterDirty: vi.fn() }))
vi.mock('./appPanels', async () => {
  const { Toggle } = await import('./primitives')
  const { NotificationsPanel } = await import('./NotificationsPanel')
  return {
    ProfilePanel: ({
      draft,
      set,
    }: {
      draft: AppSettings
      set: (key: string, value: string) => void
    }) => (
      <>
        <input
          aria-label="Name"
          value={draft.userName}
          onChange={(e) => set('userName', e.target.value)}
        />
        <select
          aria-label="Interface language"
          value={draft.uiLanguage}
          onChange={(e) => set('uiLanguage', e.target.value)}
        >
          <option value="en">English</option>
          <option value="tr">Türkçe</option>
        </select>
        <select
          aria-label="Agent reply language"
          value={draft.language}
          onChange={(e) => set('language', e.target.value)}
        >
          <option value="en">English</option>
          <option value="tr">Türkçe</option>
        </select>
      </>
    ),
    ToolsPanel: ({
      draft,
      set,
    }: {
      draft: AppSettings
      set: (key: string, value: boolean) => void
    }) => (
      <Toggle
        label="Shell"
        checked={draft.enableShell}
        onChange={(value) => set('enableShell', value)}
      />
    ),
    NotificationsPanel,
    SoundPanel: () => <p>Local sound preferences</p>,
    ContextPanel: () => null,
    BackupPanel: () => null,
    AboutPanel: () => <p>About this app</p>,
  }
})
vi.mock('./ProvidersPanel', () => ({ ProvidersPanel: () => <p>Provider connections</p> }))
vi.mock('./HooksPanel', () => ({ HooksPanel: () => null }))
vi.mock('./ExternalToolsPanel', () => ({ ExternalToolsPanel: () => null }))
vi.mock('./SecretsPanel', () => ({ SecretsPanel: () => null }))
vi.mock('./SystemAgentsPanel', () => ({ SystemAgentsPanel: () => null }))
vi.mock('@/features/decider', () => ({ DeciderPanel: () => null }))

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true
let root: ReturnType<typeof createRoot>
let container: HTMLDivElement
let saved: AppSettings
const onError = vi.fn()
const onSaved = vi.fn()

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  saved = {
    userName: 'Ada',
    enableShell: false,
    desktopNotifications: false,
    keepAwake: false,
    autoTitleEnabled: false,
    autoTagSessions: false,
    autonomousTaskBudgetTokens: 0,
  } as AppSettings
  vi.mocked(api.getSettings).mockImplementation(async () => saved)
  vi.mocked(api.getPrompts).mockResolvedValue({ dir: '', prompts: [] })
  vi.mocked(api.updateSettings).mockImplementation(async (patch) => {
    saved = { ...saved, ...patch }
    return saved
  })
  container = document.createElement('div')
  document.body.appendChild(container)
  root = createRoot(container)
})
afterEach(() => {
  act(() => root.unmount())
  container.remove()
})

async function render(cat?: string) {
  await act(async () =>
    root.render(
      <SettingsPanel
        onError={onError}
        onSaved={onSaved}
        {...(cat ? { cat, onCatChange: vi.fn() } : {})}
      />,
    ),
  )
}
async function click(text: string, scope: ParentNode = container) {
  const button = [...scope.querySelectorAll('button')].find((node) => node.textContent === text)
  expect(button, text).toBeTruthy()
  await act(async () => button!.click())
}
async function toggle(label: string) {
  await act(async () =>
    (container.querySelector(`[aria-label="${label}"]`) as HTMLButtonElement).click(),
  )
}
function markedCategories() {
  return [...container.querySelectorAll('aside button')]
    .filter((node) => node.querySelector('span[title]'))
    .map((node) => node.textContent)
}

describe('settings navigation and persistence', () => {
  it('marks and saves only the current category while retaining other drafts', async () => {
    await render()
    await click('Tool permissions')
    await toggle('Shell')
    expect(markedCategories()).toEqual(['Tool permissions'])
    await click('General')
    await toggle('Keep screen awake')
    await click('Save')
    expect(api.updateSettings).toHaveBeenCalledWith({ keepAwake: true })
    expect(markedCategories()).toEqual(['Tool permissions'])
    await click('Tool permissions')
    expect(container.querySelector('[aria-label="Shell"]')?.getAttribute('aria-checked')).toBe(
      'true',
    )
    await click('Save')
    expect(api.updateSettings).toHaveBeenLastCalledWith({ enableShell: true })
    expect(markedCategories()).toEqual([])
  })

  it.each([
    { current: 'tr', target: 'en', reply: 'tr', expected: 'Saved' },
    { current: 'en', target: 'tr', reply: 'en', expected: 'Kayıtlı' },
  ] as const)(
    'shows save feedback in target UI locale when switching $current to $target',
    async ({ current, target, reply, expected }) => {
      saved = { ...saved, uiLanguage: current, language: reply }
      await i18next.changeLanguage(current)
      await render('profile')

      const uiLanguage = container.querySelector<HTMLSelectElement>(
        '[aria-label="Interface language"]',
      )!
      await act(async () => {
        uiLanguage.value = target
        uiLanguage.dispatchEvent(new Event('change', { bubbles: true }))
      })
      await click(i18next.t('common.save', { ns: 'settings' }))

      expect(api.updateSettings).toHaveBeenCalledWith({ uiLanguage: target })
      expect(onSaved).toHaveBeenCalledWith(
        expect.objectContaining({ uiLanguage: target, language: reply }),
      )
      expect(vi.mocked(toast.success)).toHaveBeenLastCalledWith(expected)
    },
  )

  it('saves notifications immediately without losing another category draft', async () => {
    await render()
    await click('Tool permissions')
    await toggle('Shell')
    await click('Sound & notifications')
    expect(
      [...container.querySelectorAll('button')].some((button) => button.textContent === 'Save'),
    ).toBe(false)
    await toggle('Desktop notifications')
    expect(api.updateSettings).toHaveBeenCalledWith({ desktopNotifications: true })
    expect(onSaved).toHaveBeenCalledWith(expect.objectContaining({ desktopNotifications: true }))
    expect(markedCategories()).toEqual(['Tool permissions'])
  })

  it('keeps notifications unchanged and reports failed saves', async () => {
    vi.mocked(api.updateSettings).mockRejectedValueOnce(new Error('Offline'))
    await render('sound')
    await toggle('Desktop notifications')
    expect(onError).toHaveBeenCalledWith('Offline')
    expect(
      container.querySelector('[aria-label="Desktop notifications"]')?.getAttribute('aria-checked'),
    ).toBe('false')
    expect(onSaved).not.toHaveBeenCalled()
  })

  it('keeps device notification types immediate and out of the global patch', async () => {
    await render('sound')
    const switches = [...container.querySelectorAll('[role="switch"]')]
    await act(async () => (switches[1] as HTMLButtonElement).click())
    expect(localStorage.getItem('tionharness.notifyMutedTypes')).not.toBeNull()
    expect(api.updateSettings).not.toHaveBeenCalled()
  })

  it('moves legacy reference routes into help with the right content and no save indicator', async () => {
    await render('stepkinds')
    expect(container.querySelector('aside')?.textContent).toContain('Help')
    expect(container.querySelector('aside')?.textContent).not.toContain('Activity step types')
    expect(container.querySelector('[aria-pressed="true"]')?.textContent).toBe(
      'Activity step types',
    )
    expect(container.querySelector('[role="status"]')).toBeNull()
    await render('commands')
    expect(container.querySelector('[aria-pressed="true"]')?.textContent).toBe('Commands')
    await render('advanced')
    expect(container.querySelector('[aria-label="Keep screen awake"]')).not.toBeNull()
  })

  it('shows provider saving only within advanced options and blocks invalid numeric input', async () => {
    await render('providers')
    const saveButton = [...container.querySelectorAll('button')].find(
      (node) => node.textContent === 'Save',
    )!
    expect(saveButton.closest('section')?.textContent).toContain('Advanced provider options')
    const toggleButton = container.querySelector('[role="switch"]') as HTMLButtonElement
    await act(async () => toggleButton.click())
    expect(saveButton.disabled).toBe(false)
    const input = container.querySelector('input[type="number"]') as HTMLInputElement
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, '-1')
      input.dispatchEvent(new Event('input', { bubbles: true }))
    })
    expect(saveButton.disabled).toBe(true)
    expect(api.updateSettings).not.toHaveBeenCalled()
  })
})
