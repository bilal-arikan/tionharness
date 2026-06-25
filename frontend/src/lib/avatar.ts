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

// looksLikeEmoji reports whether a string contains a high-codepoint glyph (emoji,
// dingbat, pictograph). Plain ASCII/Latin-1 strings return false.
function looksLikeEmoji(s: string): boolean {
  return [...s].some((ch) => (ch.codePointAt(0) ?? 0) > 0x2000)
}

// repairMojibake tries to undo the classic corruption where UTF-8 bytes were
// mis-decoded as Latin-1 (e.g. "🗺️" stored as "ðºï¸"): re-interpret each char as
// a byte and decode the result as UTF-8. Returns null when the input is already
// a real glyph or the bytes are not valid UTF-8 (so the caller can fall back).
function repairMojibake(s: string): string | null {
  // A genuine glyph already has high code points — nothing to repair.
  if ([...s].some((ch) => (ch.codePointAt(0) ?? 0) > 0xff)) return null
  try {
    const bytes = Uint8Array.from(s, (c) => c.charCodeAt(0) & 0xff)
    return new TextDecoder('utf-8', { fatal: true }).decode(bytes)
  } catch {
    return null
  }
}

// normalizeAvatar returns a clean renderable glyph for a custom avatar, or null
// when the stored value is unusable. It accepts genuine emoji as-is, repairs
// mojibake when it can, and rejects anything else (so callers show initials
// instead of garbage boxes).
export function normalizeAvatar(avatar: string | undefined | null): string | null {
  const a = avatar?.trim()
  if (!a) return null
  if (looksLikeEmoji(a)) return a
  const repaired = repairMojibake(a)
  if (repaired && looksLikeEmoji(repaired)) return repaired
  return null
}

// glyph returns what to render inside the circle: the custom emoji if present and
// valid (mojibake repaired when possible), otherwise the name initials.
export function avatarGlyph(agent: Pick<Agent, 'name' | 'avatar'>): string {
  return normalizeAvatar(agent.avatar) ?? initials(agent.name)
}
