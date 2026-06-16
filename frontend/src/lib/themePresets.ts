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

// The order here is the order shown in the Settings appearance picker.
export const THEME_PRESETS: ThemePreset[] = [
  {
    id: 'midnight-violet',
    label: 'Gece Moru',
    dark: true,
    tokens: {
      bg: '#0c0c10',
      surface: '#161619',
      surface2: '#212129',
      border: '#2a2a33',
      accent: '#8b5cf6',
      accentSoft: '#2c2545',
      text: '#e7e7ea',
      textDim: '#9a9aa6',
    },
  },
  {
    id: 'slate',
    label: 'Arduvaz',
    dark: true,
    tokens: {
      bg: '#0d1117',
      surface: '#161b22',
      surface2: '#21262d',
      border: '#30363d',
      accent: '#58a6ff',
      accentSoft: '#16304d',
      text: '#e6edf3',
      textDim: '#8b949e',
    },
  },
  {
    id: 'emerald',
    label: 'Zümrüt',
    dark: true,
    tokens: {
      bg: '#0a0f0d',
      surface: '#111815',
      surface2: '#1a241f',
      border: '#25332c',
      accent: '#34d399',
      accentSoft: '#123528',
      text: '#e6efe9',
      textDim: '#92a89c',
    },
  },
  {
    id: 'rose',
    label: 'Gül',
    dark: true,
    tokens: {
      bg: '#100c0e',
      surface: '#1a1518',
      surface2: '#251c20',
      border: '#33252d',
      accent: '#fb7185',
      accentSoft: '#3a1f29',
      text: '#f0e7ea',
      textDim: '#a89aa0',
    },
  },
  {
    id: 'amber',
    label: 'Kehribar',
    dark: true,
    tokens: {
      bg: '#100d08',
      surface: '#191510',
      surface2: '#241e16',
      border: '#332a1d',
      accent: '#f59e0b',
      accentSoft: '#3a2a12',
      text: '#efe9df',
      textDim: '#a89f8d',
    },
  },
  {
    id: 'nord',
    label: 'Nord',
    dark: true,
    tokens: {
      bg: '#242933',
      surface: '#2e3440',
      surface2: '#3b4252',
      border: '#434c5e',
      accent: '#88c0d0',
      accentSoft: '#3b4a55',
      text: '#eceff4',
      textDim: '#a3acbd',
    },
  },
  {
    id: 'daylight',
    label: 'Gün Işığı',
    dark: false,
    tokens: {
      bg: '#f6f8fb',
      surface: '#ffffff',
      surface2: '#eef1f6',
      border: '#d7dde6',
      accent: '#2f6fed',
      accentSoft: '#d8e4fb',
      text: '#16202c',
      textDim: '#5a6b7d',
    },
  },
  {
    id: 'solarized-light',
    label: 'Solarized Açık',
    dark: false,
    tokens: {
      bg: '#fdf6e3',
      surface: '#fbf3df',
      surface2: '#eee8d5',
      border: '#ddd6c1',
      accent: '#268bd2',
      accentSoft: '#d8e7f0',
      text: '#073642',
      textDim: '#657b83',
    },
  },
]

export const presetById = (id: string): ThemePreset | undefined =>
  THEME_PRESETS.find((p) => p.id === id)
