// Deterministic visual identity for agent avatars. When an agent has no custom
// color, we derive a stable hue from its id so the same agent always renders
// the same circle across reloads.
import type { Agent } from '../types'

// A curated palette of accent colors offered in the avatar picker. The first
// entry is treated as "auto" (derive from id) when stored as an empty string.
export const AVATAR_COLORS = [
  '#7c3aed', // violet
  '#2563eb', // blue
  '#0891b2', // cyan
  '#059669', // emerald
  '#65a30d', // lime
  '#ca8a04', // amber
  '#ea580c', // orange
  '#dc2626', // red
  '#db2777', // pink
  '#475569', // slate
] as const

// Small set of suggested glyphs for quick selection in the editor.
export const AVATAR_GLYPHS = [
  '🤖', '🧠', '🛰️', '⚙️', '🔭', '🧭', '📡', '🦾',
  '🧪', '📊', '✍️', '🎯', '🔮', '🐝', '🦉', '🐙',
] as const

// hashString folds a string into a small non-negative integer (djb2).
function hashString(s: string): number {
  let h = 5381
  for (let i = 0; i < s.length; i++) {
    h = ((h << 5) + h + s.charCodeAt(i)) | 0
  }
  return Math.abs(h)
}

// resolveColor returns the agent's explicit color, or a deterministic palette
// pick derived from its id when none is set.
export function resolveColor(agent: Pick<Agent, 'id' | 'color'>): string {
  if (agent.color) return agent.color
  return AVATAR_COLORS[hashString(agent.id || agent.color || '') % AVATAR_COLORS.length]
}

// initials returns up to two uppercase letters for the fallback avatar glyph.
export function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return '?'
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase()
  return (parts[0][0] + parts[1][0]).toUpperCase()
}

// glyph returns what to render inside the circle: the custom emoji if present,
// otherwise the name initials.
export function avatarGlyph(agent: Pick<Agent, 'name' | 'avatar'>): string {
  if (agent.avatar) return agent.avatar
  return initials(agent.name)
}
