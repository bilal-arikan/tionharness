/**
 * Mirrors frontend/src/shared/lib/themePresets.ts. Six colour families, each with
 * a dark and a light variant, sharing a constant neutral ramp -- switching a
 * colour only repaints the accent, never the canvas hue.
 *
 * Keep in sync when the app's preset list changes.
 */

export interface Neutrals {
  bg: string
  surface: string
  surface2: string
  border: string
  text: string
  textDim: string
}

export const DARK_NEUTRALS: Neutrals = {
  bg: '#0e0e10',
  surface: '#17171a',
  surface2: '#202024',
  border: '#2b2b30',
  text: '#e7e7ea',
  textDim: '#9a9aa6',
}

export const LIGHT_NEUTRALS: Neutrals = {
  bg: '#f5f6f8',
  surface: '#ffffff',
  surface2: '#eceef2',
  border: '#d8dce3',
  text: '#16202c',
  textDim: '#5a6470',
}

export interface ThemeColor {
  id: string
  label: string
  darkAccent: string
  darkSoft: string
  lightAccent: string
  lightSoft: string
}

export const themeColors: ThemeColor[] = [
  { id: 'violet', label: 'Violet', darkAccent: '#8b5cf6', darkSoft: '#2c2545', lightAccent: '#7c3aed', lightSoft: '#ece7fb' },
  { id: 'blue', label: 'Blue', darkAccent: '#58a6ff', darkSoft: '#16304d', lightAccent: '#2f6fed', lightSoft: '#d8e4fb' },
  { id: 'emerald', label: 'Emerald', darkAccent: '#34d399', darkSoft: '#123528', lightAccent: '#15915b', lightSoft: '#d6f0e3' },
  { id: 'rose', label: 'Rose', darkAccent: '#fb7185', darkSoft: '#3a1f29', lightAccent: '#e11d48', lightSoft: '#fbe0e6' },
  { id: 'amber', label: 'Amber', darkAccent: '#f59e0b', darkSoft: '#3a2a12', lightAccent: '#b45309', lightSoft: '#f7e6cf' },
  { id: 'nord', label: 'Nord', darkAccent: '#88c0d0', darkSoft: '#2b3d44', lightAccent: '#3b7e93', lightSoft: '#d9eaf0' },
]
