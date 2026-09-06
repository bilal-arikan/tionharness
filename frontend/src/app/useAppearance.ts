// useAppearance owns the client-side preference plumbing: theme resolution
// (global default + per-workspace override), keep-awake and desktop-notification
// toggles. Appearance is per-workspace: the app-global appearance is the
// inherited default, and each workspace may override the theme preset. Both live
// in refs so a global-settings save and a workspace switch can each re-resolve
// and re-apply the effective theme without racing each other.
import { useCallback, useEffect, useRef } from 'react'
import { api } from '@/api'
import { applyAppearance, resolveAppearance, type Appearance } from '@/shared/lib/theme'
import { applyKeepAwake, ensureNotificationPermission } from '@/shared/lib/clientPrefs'
import { setGuardNoteVisible } from '@/shared/lib/coordinationGuardNote'
import { resolveUILocale, setLocale } from '@/i18n'

export interface ClientPrefs {
  themePreset?: string
  keepAwake: boolean
  desktopNotifications: boolean
  // UI language, plus the agent reply language it falls back to when unset. Both
  // are app-global (not per-workspace): a single interface should not change
  // language as the user switches workspaces.
  uiLanguage: string
  language: string
  // Display-only: render the runtime's <coordination-guard> notes in the
  // transcript. Off by default; the note is recorded either way.
  coordinatorStallNoteVisible: boolean
}

export function useAppearance(activeWorkspaceId: string | null, setError: (msg: string) => void) {
  // Desktop-notification master toggle, read live in callbacks without
  // re-binding.
  const notifyEnabled = useRef(false)
  const globalNotifyRef = useRef(false)
  const applyResolvedNotify = useCallback(() => {
    const enabled = globalNotifyRef.current
    ensureNotificationPermission(enabled)
    notifyEnabled.current = enabled
  }, [])
  const globalAppearanceRef = useRef<Appearance>({ themePreset: 'violet-dark' })
  const wsAppearanceRef = useRef<Partial<Appearance> | null>(null)
  const applyResolvedTheme = useCallback(() => {
    applyAppearance(resolveAppearance(wsAppearanceRef.current, globalAppearanceRef.current))
  }, [])

  // Apply the client-side preferences carried by app settings. Theme resolution
  // honors the active workspace's override on top of these global defaults.
  const applyClientPrefs = useCallback(
    (s: ClientPrefs) => {
      globalAppearanceRef.current = { themePreset: s.themePreset ?? '' }
      applyResolvedTheme()
      // Language is applied from the same settings payload that already syncs
      // theme across windows, so an agent's update_settings or another window's
      // save switches the interface language live via the `settings` SSE event.
      void setLocale(resolveUILocale(s.uiLanguage ?? '', s.language ?? ''))
      applyKeepAwake(s.keepAwake)
      globalNotifyRef.current = s.desktopNotifications
      applyResolvedNotify()
      setGuardNoteVisible(s.coordinatorStallNoteVisible)
    },
    [applyResolvedTheme, applyResolvedNotify],
  )

  // onAppearanceSaved is invoked by the Settings "Görünüm" panel after it persists
  // the active workspace's appearance override, so the ref + the live theme stay
  // in sync (a later global save must not clobber the workspace choice).
  const onAppearanceSaved = useCallback(
    (a: Partial<Appearance>) => {
      wsAppearanceRef.current = a
      applyResolvedTheme()
    },
    [applyResolvedTheme],
  )

  // Load global settings once and apply theme + client-side behaviours.
  useEffect(() => {
    api
      .getSettings()
      .then(applyClientPrefs)
      .catch((e) => setError(e.message))
  }, [applyClientPrefs, setError])

  // Re-theme whenever the active workspace changes: fetch that workspace's
  // appearance override and apply it on top of the global defaults.
  useEffect(() => {
    if (!activeWorkspaceId) return
    let cancelled = false
    api
      .getWorkspaceSettings()
      .then((w) => {
        if (cancelled) return
        wsAppearanceRef.current = { themePreset: w.themePreset }
        applyResolvedTheme()
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [activeWorkspaceId, applyResolvedTheme])

  return { applyClientPrefs, onAppearanceSaved, notifyEnabled }
}
