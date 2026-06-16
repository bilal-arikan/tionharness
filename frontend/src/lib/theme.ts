import type { AppSettings } from '../types'

// applyTheme reflects the theme + accent settings onto the document root so the
// CSS custom properties re-theme the whole UI. "system" resolves via the OS
// color-scheme preference.
export function applyTheme(theme: AppSettings['theme'], accent: string) {
  const resolved =
    theme === 'system'
      ? window.matchMedia('(prefers-color-scheme: light)').matches
        ? 'light'
        : 'dark'
      : theme

  const root = document.documentElement
  if (resolved === 'light') {
    root.setAttribute('data-theme', 'light')
  } else {
    root.removeAttribute('data-theme')
  }
  if (accent) {
    root.style.setProperty('--color-accent', accent)
  }
}
