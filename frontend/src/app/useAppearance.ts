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
import { resolveDesktopNotifications } from '@/shared/lib/desktopNotifications'
import type { DesktopNotificationsMode } from '@/types/workspace'

export interface ClientPrefs {
  themePreset?: string
  keepAwake: boolean
  desktopNotifications: boolean
}

export function useAppearance(
  activeWorkspaceId: string | null,
  setError: (msg: string) => void,
) {
  // Effective desktop-notification preference (global master ⊕ active-workspace
  // override), read live in callbacks without re-binding. The two source values
  // live in their own refs so a global-settings save and a workspace switch can
  // each re-resolve the effective gate independently, mirroring the theme refs.
  const notifyEnabled = useRef(false)
  const globalNotifyRef = useRef(false)
  const wsNotifyModeRef = useRef<DesktopNotificationsMode>('inherit')
  const applyResolvedNotify = useCallback(() => {
    const enabled = resolveDesktopNotifications(wsNotifyModeRef.current, globalNotifyRef.current)
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
  const applyClientPrefs = useCallback((s: ClientPrefs) => {
    globalAppearanceRef.current = { themePreset: s.themePreset ?? '' }
    applyResolvedTheme()
    applyKeepAwake(s.keepAwake)
    globalNotifyRef.current = s.desktopNotifications
    applyResolvedNotify()
  }, [applyResolvedTheme, applyResolvedNotify])

  // onAppearanceSaved is invoked by the Settings "Görünüm" panel after it persists
  // the active workspace's appearance override, so the ref + the live theme stay
  // in sync (a later global save must not clobber the workspace choice).
  const onAppearanceSaved = useCallback((a: Partial<Appearance>) => {
    wsAppearanceRef.current = a
    applyResolvedTheme()
  }, [applyResolvedTheme])

  // onWorkspaceNotifySaved is invoked by the Settings "Bildirimler" panel after it
  // persists this workspace's desktopNotifications override, so the effective gate
  // updates live without waiting for a workspace switch or a global-settings reload.
  const onWorkspaceNotifySaved = useCallback((mode: DesktopNotificationsMode) => {
    wsNotifyModeRef.current = mode
    applyResolvedNotify()
  }, [applyResolvedNotify])

  // Load global settings once and apply theme + client-side behaviours.
  useEffect(() => {
    api.getSettings().then(applyClientPrefs).catch((e) => setError(e.message))
  }, [applyClientPrefs, setError])

  // Re-theme whenever the active workspace changes: fetch that workspace's
  // appearance override and apply it on top of the global defaults.
  useEffect(() => {
    if (!activeWorkspaceId) return
    let cancelled = false
    api.getWorkspaceSettings()
      .then((w) => {
        if (cancelled) return
        wsAppearanceRef.current = { themePreset: w.themePreset }
        applyResolvedTheme()
        wsNotifyModeRef.current = w.desktopNotifications
        applyResolvedNotify()
      })
      .catch(() => {})
    return () => { cancelled = true }
  }, [activeWorkspaceId, applyResolvedTheme, applyResolvedNotify])

  return { applyClientPrefs, onAppearanceSaved, onWorkspaceNotifySaved, notifyEnabled }
}
