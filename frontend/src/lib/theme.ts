import type { AppSettings } from '../types'
import { presetById, type PresetTokens } from './themePresets'

// The full set of palette custom properties a preset controls. Kept in one place
// so switching to legacy mode can clear exactly what a preset set inline.
const PALETTE_VARS = [
  '--color-bg',
  '--color-surface',
  '--color-surface-2',
  '--color-border',
  '--color-accent',
  '--color-accent-soft',
  '--color-text',
  '--color-text-dim',
  '--color-success',
  '--color-warning',
  '--color-danger',
] as const

function setPalette(root: HTMLElement, t: PresetTokens, dark: boolean) {
  root.style.setProperty('--color-bg', t.bg)
  root.style.setProperty('--color-surface', t.surface)
  root.style.setProperty('--color-surface-2', t.surface2)
  root.style.setProperty('--color-border', t.border)
  root.style.setProperty('--color-accent', t.accent)
  root.style.setProperty('--color-accent-soft', t.accentSoft)
  root.style.setProperty('--color-text', t.text)
  root.style.setProperty('--color-text-dim', t.textDim)
  // Semantic colours: presets may override, else a tasteful per-mode default.
  root.style.setProperty('--color-success', t.success ?? (dark ? '#34d399' : '#15915b'))
  root.style.setProperty('--color-warning', t.warning ?? (dark ? '#fbbf24' : '#b45309'))
  root.style.setProperty('--color-danger', t.danger ?? (dark ? '#f87171' : '#dc2626'))
}

function clearPalette(root: HTMLElement) {
  for (const v of PALETTE_VARS) root.style.removeProperty(v)
}

// applyTheme reflects the appearance settings onto the document root so the CSS
// custom properties re-theme the whole UI.
//
// Resolution order:
//   1. A curated preset (themePreset) wins — it applies a full palette inline
//      and sets data-theme from its dark/light nature.
//   2. Otherwise the legacy theme (dark/light/system) + accent path is used.
// In both cases a non-empty `accent` further overrides the accent token so the
// custom colour picker keeps working on top of any preset.
export function applyTheme(
  theme: AppSettings['theme'],
  accent: string,
  preset?: string,
) {
  const root = document.documentElement
  const p = preset ? presetById(preset) : undefined

  if (p) {
    setPalette(root, p.tokens, p.dark)
    if (p.dark) root.removeAttribute('data-theme')
    else root.setAttribute('data-theme', 'light')
  } else {
    clearPalette(root)
    const resolved =
      theme === 'system'
        ? window.matchMedia('(prefers-color-scheme: light)').matches
          ? 'light'
          : 'dark'
        : theme
    if (resolved === 'light') root.setAttribute('data-theme', 'light')
    else root.removeAttribute('data-theme')
  }

  if (accent) root.style.setProperty('--color-accent', accent)
}

// Appearance is the visual subset of settings that can be overridden per
// workspace. The app-global values act as the inherited defaults.
export interface Appearance {
  theme: AppSettings['theme']
  accent: string
  themePreset: string
}

// resolveAppearance merges a workspace's appearance override onto the global
// appearance: each empty field inherits the global value. This is what drives
// the UI re-theming itself when the active workspace changes — a workspace with
// no overrides looks exactly like the global default.
export function resolveAppearance(
  ws: Partial<Appearance> | null | undefined,
  global: Appearance,
): Appearance {
  return {
    theme: (ws?.theme as AppSettings['theme']) || global.theme,
    accent: ws?.accent || global.accent,
    themePreset: ws?.themePreset || global.themePreset,
  }
}

// applyAppearance applies a resolved appearance to the document root.
export function applyAppearance(a: Appearance) {
  applyTheme(a.theme, a.accent, a.themePreset)
}
