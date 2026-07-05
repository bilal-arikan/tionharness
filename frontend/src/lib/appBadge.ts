// setAppBadge reflects the unread count on the OS taskbar/dock icon. It uses the
// Chromium Badging API (navigator.setAppBadge), which works in Edge/WebView2 —
// so the native desktop window gets a taskbar overlay badge for free. An optional
// host bridge (window.tionswarmSetBadge) is also called when the desktop shell
// exposes one. All calls are best-effort: unsupported environments no-op.
export function setAppBadge(count: number) {
  const nav = navigator as Navigator & {
    setAppBadge?: (n?: number) => Promise<void>
    clearAppBadge?: () => Promise<void>
  }
  try {
    if (count > 0) void nav.setAppBadge?.(count)
    else void nav.clearAppBadge?.()
  } catch {
    /* Badging API unsupported — ignore */
  }
  const host = window as unknown as { tionswarmSetBadge?: (n: number) => void }
  try {
    host.tionswarmSetBadge?.(count)
  } catch {
    /* no native bridge — ignore */
  }
}
