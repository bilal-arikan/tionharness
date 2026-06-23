// useUnreadBadge surfaces the total unread count outside the app window: the tab
// title shows "(N) SwarmGo" while the window is UNFOCUSED (so a background window
// signals pending attention), and the OS taskbar/dock icon gets a numeric badge
// via the Badging API regardless of focus. Returns nothing — it is a side effect.
import { useEffect } from 'react'
import { setAppBadge } from '../lib/appBadge'

const BASE_TITLE = 'SwarmGo'

export function useUnreadBadge(count: number) {
  // Tab title: only prefix the count while the window is not focused — a focused
  // window is already "seen", so its title stays clean.
  useEffect(() => {
    const apply = () => {
      const show = count > 0 && !document.hasFocus()
      document.title = show ? `(${count}) ${BASE_TITLE}` : BASE_TITLE
    }
    apply()
    window.addEventListener('focus', apply)
    window.addEventListener('blur', apply)
    document.addEventListener('visibilitychange', apply)
    return () => {
      window.removeEventListener('focus', apply)
      window.removeEventListener('blur', apply)
      document.removeEventListener('visibilitychange', apply)
      document.title = BASE_TITLE
    }
  }, [count])

  // Taskbar/dock badge reflects pending items regardless of focus.
  useEffect(() => {
    setAppBadge(count)
  }, [count])
}
