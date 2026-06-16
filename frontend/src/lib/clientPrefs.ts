// Client-side behaviours driven by settings: a screen Wake Lock and desktop
// notifications. Both degrade gracefully where the browser lacks the API.

let wakeLock: WakeLockSentinel | null = null
let keepAwakeOn = false

async function acquireWakeLock() {
  if (!keepAwakeOn) return
  try {
    // navigator.wakeLock is not in older TS DOM libs; guard at runtime.
    const nav = navigator as Navigator & { wakeLock?: { request: (t: 'screen') => Promise<WakeLockSentinel> } }
    if (!nav.wakeLock) return
    wakeLock = await nav.wakeLock.request('screen')
    wakeLock.addEventListener?.('release', () => { wakeLock = null })
  } catch {
    // permission denied / not supported — ignore.
  }
}

// The browser releases the wake lock when the tab is hidden; re-acquire on
// return to keep the screen awake across visibility changes.
function onVisible() {
  if (keepAwakeOn && document.visibilityState === 'visible' && !wakeLock) {
    void acquireWakeLock()
  }
}

let visibilityBound = false

// applyKeepAwake turns the screen wake lock on or off to match the setting.
export function applyKeepAwake(enabled: boolean) {
  keepAwakeOn = enabled
  if (!visibilityBound) {
    document.addEventListener('visibilitychange', onVisible)
    visibilityBound = true
  }
  if (enabled) {
    void acquireWakeLock()
  } else {
    wakeLock?.release?.().catch(() => {})
    wakeLock = null
  }
}

// ensureNotificationPermission requests permission when notifications are turned
// on so the first real notification can be shown without delay.
export function ensureNotificationPermission(enabled: boolean) {
  if (!enabled || !('Notification' in window)) return
  if (Notification.permission === 'default') {
    void Notification.requestPermission()
  }
}

// notify shows a desktop notification when enabled, permitted, and the app is in
// the background (no point notifying a focused window). An optional onClick runs
// when the user clicks the notification (after focusing the window) so callers
// can navigate to the relevant target, e.g. the source chat or the logs view.
export function notify(enabled: boolean, title: string, body: string, onClick?: () => void) {
  if (!enabled || !('Notification' in window)) return
  if (Notification.permission !== 'granted') return
  if (document.visibilityState === 'visible') return
  try {
    const n = new Notification(title, { body: body.slice(0, 180) })
    if (onClick) {
      n.onclick = () => {
        // Bring the app to the foreground, then run the navigation callback.
        try { window.focus() } catch { /* ignore */ }
        onClick()
        n.close()
      }
    }
  } catch {
    // ignore
  }
}
