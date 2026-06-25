// Curated theme presets. Each preset is a complete palette that maps onto the
// CSS custom properties consumed across the UI (var(--color-*)). Because every
// component references those tokens, swapping a preset re-themes the whole app.
//
// Selecting a preset applies its tokens as inline styles on <html>, which win
// over the stylesheet defaults in index.css. Clearing the preset ("") restores
// the legacy theme + accent behaviour driven by index.css.

export interface PresetTokens {
  bg: string
  surface: string
  surface2: string
  border: string
  accent: string
  accentSoft: string
  text: string
  textDim: string
  // Optional semantic overrides; sensible per-mode defaults are used otherwise.
  success?: string
  warning?: string
  danger?: string
}

export interface ThemePreset {
  id: string
  label: string
  dark: boolean
  tokens: PresetTokens
}

// Neutral background ramps shared by every preset of a given mode. The page
// canvas (bg/surface/surface2/border) and the text colours stay a constant
// near-grey so switching the accent/preset only repaints the accent — not the
// whole background hue. (Previously each preset tinted its backgrounds with its
// accent colour, so every colour change shifted the entire canvas and tired the
// eyes.) Each preset only contributes its accent + accentSoft on top of these.
type Neutrals = Pick<PresetTokens, 'bg' | 'surface' | 'surface2' | 'border' | 'text' | 'textDim'>

const DARK_NEUTRALS: Neutrals = {
  bg: '#0e0e10',
  surface: '#17171a',
  surface2: '#202024',
  border: '#2b2b30',
  text: '#e7e7ea',
  textDim: '#9a9aa6',
}

const LIGHT_NEUTRALS: Neutrals = {
  bg: '#f5f6f8',
  surface: '#ffffff',
  surface2: '#eceef2',
  border: '#d8dce3',
  text: '#16202c',
  textDim: '#5a6470',
}

// The order here is the order shown in the Settings appearance picker. Only the
// accent (+ its soft selected-surface tint) differs between same-mode presets.
export const THEME_PRESETS: ThemePreset[] = [
  {
    id: 'midnight-violet',
    label: 'Gece Moru',
    dark: true,
    tokens: { ...DARK_NEUTRALS, accent: '#8b5cf6', accentSoft: '#2c2545' },
  },
  {
    id: 'slate',
    label: 'Arduvaz',
    dark: true,
    tokens: { ...DARK_NEUTRALS, accent: '#58a6ff', accentSoft: '#16304d' },
  },
  {
    id: 'emerald',
    label: 'Zümrüt',
    dark: true,
    tokens: { ...DARK_NEUTRALS, accent: '#34d399', accentSoft: '#123528' },
  },
  {
    id: 'rose',
    label: 'Gül',
    dark: true,
    tokens: { ...DARK_NEUTRALS, accent: '#fb7185', accentSoft: '#3a1f29' },
  },
  {
    id: 'amber',
    label: 'Kehribar',
    dark: true,
    tokens: { ...DARK_NEUTRALS, accent: '#f59e0b', accentSoft: '#3a2a12' },
  },
  {
    id: 'nord',
    label: 'Nord',
    dark: true,
    tokens: { ...DARK_NEUTRALS, accent: '#88c0d0', accentSoft: '#2b3d44' },
  },
  {
    id: 'daylight',
    label: 'Gün Işığı',
    dark: false,
    tokens: { ...LIGHT_NEUTRALS, accent: '#2f6fed', accentSoft: '#d8e4fb' },
  },
  {
    id: 'solarized-light',
    label: 'Solarized Açık',
    dark: false,
    tokens: { ...LIGHT_NEUTRALS, accent: '#268bd2', accentSoft: '#d8e7f0' },
  },
]

export const presetById = (id: string): ThemePreset | undefined =>
  THEME_PRESETS.find((p) => p.id === id)
