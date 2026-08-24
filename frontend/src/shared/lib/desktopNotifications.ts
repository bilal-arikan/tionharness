// resolveDesktopNotifications collapses the app-global desktop-notification master
// toggle and a workspace's three-state override into the single effective boolean
// that gates every OS toast (see useAppearance's notifyEnabled ref).
//
// The workspace override wins unless it is 'inherit', in which case the global
// value applies. This keeps notifications workspace-scoped — a user can silence a
// background "autonomous" workspace while keeping them on for the one they work in
// — consistent with TionHarness's physical workspace isolation, while defaulting to
// the pre-existing global behaviour for any workspace that has not opted in.
import type { DesktopNotificationsMode } from '@/types/workspace'

export function resolveDesktopNotifications(
  workspaceMode: DesktopNotificationsMode | undefined,
  globalEnabled: boolean,
): boolean {
  switch (workspaceMode) {
    case 'on':
      return true
    case 'off':
      return false
    default:
      // 'inherit' or undefined (workspace settings not yet loaded) → follow global.
      return globalEnabled
  }
}
