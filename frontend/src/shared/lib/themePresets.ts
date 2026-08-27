// Curated theme colors. Each color is a family with a DARK and a LIGHT variant;
// every variant is a complete palette mapped onto the CSS custom properties
// consumed across the UI (var(--color-*)). Because every component references
// those tokens, selecting a variant re-themes the whole app.
//
// There is no separate light/dark "base mode" anymore: the chosen variant id
// (e.g. "violet-dark" / "violet-light") encodes both the color and the mode.
// Selecting one applies its tokens as inline styles on <html>, which win over
// the stylesheet defaults in index.css.

export interface PresetTokens {
  bg: string
  surface: string
  surface2: string
  border: string
  accent: string
  accentSoft: string
  onAccent: string
  text: string
  textDim: string
  success: string
  onSuccess: string
  warning: string
  onWarning: string
  danger: string
}

export interface ThemePreset {
  id: string
  label: string
  dark: boolean
  tokens: PresetTokens
}

// Neutral background ramps shared by every variant of a given mode. The page
// canvas (bg/surface/surface2/border) and the text colours stay a constant
// near-grey so switching the color only repaints the accent — not the whole
// background hue. Each color only contributes its accent + accentSoft on top.
type Neutrals = Pick<
  PresetTokens,
  | 'bg'
  | 'surface'
  | 'surface2'
  | 'border'
  | 'text'
  | 'textDim'
  | 'success'
  | 'onSuccess'
  | 'warning'
  | 'onWarning'
  | 'danger'
>

const DARK_NEUTRALS: Neutrals = {
  bg: '#0e0e10',
  surface: '#17171a',
  surface2: '#202024',
  border: '#2b2b30',
  text: '#e7e7ea',
  textDim: '#9a9aa6',
  success: '#34d399',
  onSuccess: '#000000',
  warning: '#fbbf24',
  onWarning: '#000000',
  danger: '#f87171',
}

const LIGHT_NEUTRALS: Neutrals = {
  bg: '#f5f6f8',
  surface: '#ffffff',
  surface2: '#eceef2',
  border: '#d8dce3',
  text: '#16202c',
  textDim: '#5a6470',
  success: '#12784b',
  onSuccess: '#ffffff',
  warning: '#a54a07',
  onWarning: '#ffffff',
  danger: '#b91c1c',
}

// One entry per color family. Each carries the accent (+ soft selected-surface
// tint) for both its dark and light variant. Light accents are a touch deeper so
// they keep enough contrast on the light canvas.
interface ColorDef {
  id: string
  label: string
  darkAccent: string
  darkSoft: string
  darkOnAccent: string
  lightAccent: string
  lightSoft: string
  lightOnAccent: string
}

const COLORS: ColorDef[] = [
  {
    id: 'violet',
    label: 'Mor',
    darkAccent: '#8b5cf6',
    darkSoft: '#2c2545',
    darkOnAccent: '#000000',
    lightAccent: '#7c3aed',
    lightSoft: '#ece7fb',
    lightOnAccent: '#ffffff',
  },
  {
    id: 'blue',
    label: 'Mavi',
    darkAccent: '#58a6ff',
    darkSoft: '#16304d',
    darkOnAccent: '#000000',
    lightAccent: '#2f6fed',
    lightSoft: '#d8e4fb',
    lightOnAccent: '#ffffff',
  },
  {
    id: 'emerald',
    label: 'Zümrüt',
    darkAccent: '#34d399',
    darkSoft: '#123528',
    darkOnAccent: '#000000',
    lightAccent: '#15915b',
    lightSoft: '#d6f0e3',
    lightOnAccent: '#000000',
  },
  {
    id: 'rose',
    label: 'Gül',
    darkAccent: '#fb7185',
    darkSoft: '#3a1f29',
    darkOnAccent: '#000000',
    lightAccent: '#e11d48',
    lightSoft: '#fbe0e6',
    lightOnAccent: '#ffffff',
  },
  {
    id: 'amber',
    label: 'Kehribar',
    darkAccent: '#f59e0b',
    darkSoft: '#3a2a12',
    darkOnAccent: '#000000',
    lightAccent: '#b45309',
    lightSoft: '#f7e6cf',
    lightOnAccent: '#ffffff',
  },
  {
    id: 'nord',
    label: 'Nord',
    darkAccent: '#88c0d0',
    darkSoft: '#2b3d44',
    darkOnAccent: '#000000',
    lightAccent: '#3b7e93',
    lightSoft: '#d9eaf0',
    lightOnAccent: '#ffffff',
  },
]

// The default applied when nothing is selected yet.
export const DEFAULT_PRESET = 'violet-dark'

// Flat preset list (two entries per color: dark + light), keyed by id. This is
// what applyTheme resolves a stored themePreset id against.
export const THEME_PRESETS: ThemePreset[] = COLORS.flatMap((c) => [
  {
    id: `${c.id}-dark`,
    label: c.label,
    dark: true,
    tokens: {
      ...DARK_NEUTRALS,
      accent: c.darkAccent,
      accentSoft: c.darkSoft,
      onAccent: c.darkOnAccent,
    },
  },
  {
    id: `${c.id}-light`,
    label: c.label,
    dark: false,
    tokens: {
      ...LIGHT_NEUTRALS,
      accent: c.lightAccent,
      accentSoft: c.lightSoft,
      onAccent: c.lightOnAccent,
    },
  },
])

// One swatch's display info for the appearance picker.
export interface ThemeColorVariant {
  id: string // preset id (e.g. "violet-dark")
  accent: string
  bg: string
  border: string
}

// A color family with both its variants — the unit the appearance picker shows
// as a row (label + a Light swatch + a Dark swatch).
export interface ThemeColor {
  id: string
  label: string
  dark: ThemeColorVariant
  light: ThemeColorVariant
}

export const THEME_COLORS: ThemeColor[] = COLORS.map((c) => ({
  id: c.id,
  label: c.label,
  dark: {
    id: `${c.id}-dark`,
    accent: c.darkAccent,
    bg: DARK_NEUTRALS.bg,
    border: DARK_NEUTRALS.border,
  },
  light: {
    id: `${c.id}-light`,
    accent: c.lightAccent,
    bg: LIGHT_NEUTRALS.bg,
    border: LIGHT_NEUTRALS.border,
  },
}))

export const presetById = (id: string): ThemePreset | undefined =>
  THEME_PRESETS.find((p) => p.id === id)
