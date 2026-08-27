import { presetById, DEFAULT_PRESET, type PresetTokens } from './themePresets'

function setPalette(root: HTMLElement, t: PresetTokens) {
  root.style.setProperty('--color-bg', t.bg)
  root.style.setProperty('--color-surface', t.surface)
  root.style.setProperty('--color-surface-2', t.surface2)
  root.style.setProperty('--color-border', t.border)
  root.style.setProperty('--color-sender-bubble', t.senderBubble)
  root.style.setProperty('--color-on-sender-bubble', t.onSenderBubble)
  root.style.setProperty('--color-sender-bubble-border', t.senderBubbleBorder)
  root.style.setProperty('--color-accent', t.accent)
  root.style.setProperty('--color-accent-soft', t.accentSoft)
  root.style.setProperty('--color-on-accent', t.onAccent)
  root.style.setProperty('--color-text', t.text)
  root.style.setProperty('--color-text-dim', t.textDim)
  root.style.setProperty('--color-success', t.success)
  root.style.setProperty('--color-on-success', t.onSuccess)
  root.style.setProperty('--color-warning', t.warning)
  root.style.setProperty('--color-on-warning', t.onWarning)
  root.style.setProperty('--color-danger', t.danger)
}

// applyTheme reflects the selected theme preset onto the document root so the CSS
// custom properties re-theme the whole UI. The preset id encodes both the color
// and the mode (its dark/light nature drives data-theme) — there is no separate
// base-mode or accent override anymore. An empty/unknown id falls back to the
// default preset so the UI is always fully themed.
export function applyTheme(preset?: string) {
  const root = document.documentElement
  const p = presetById(preset || DEFAULT_PRESET) ?? presetById(DEFAULT_PRESET)!
  setPalette(root, p.tokens)
  if (p.dark) root.removeAttribute('data-theme')
  else root.setAttribute('data-theme', 'light')
}

// Appearance is the visual subset of settings that can be overridden per
// workspace. The app-global value acts as the inherited default.
export interface Appearance {
  themePreset: string
}

// resolveAppearance merges a workspace's appearance override onto the global
// appearance: an empty field inherits the global value. This is what drives the
// UI re-theming itself when the active workspace changes — a workspace with no
// override looks exactly like the global default.
export function resolveAppearance(
  ws: Partial<Appearance> | null | undefined,
  global: Appearance,
): Appearance {
  return {
    themePreset: ws?.themePreset || global.themePreset,
  }
}

// applyAppearance applies a resolved appearance to the document root.
export function applyAppearance(a: Appearance) {
  applyTheme(a.themePreset)
}
