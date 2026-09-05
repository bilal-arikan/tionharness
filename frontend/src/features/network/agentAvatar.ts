// Agent avatar rendering for vis-network nodes. Lived in relationGraph.ts while
// the Ağ screen existed; the Explorer map's live layer is now its only user.
import { avatarForeground } from '@/shared/lib/avatar'

// initialsAscii returns a 1-2 char ASCII fallback for an agent's avatar when no
// emoji is stored. A high-codepoint emoji glyph is treated as already-rendered
// and bypassed (we don't want to half-show a broken glyph).
export function initialsAscii(label: string): string {
  const parts = label.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return '?'
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase()
  return (parts[0][0] + parts[1][0]).toUpperCase()
}

// agentAvatarDataUrl renders an SVG circular avatar (color-filled disc with
// the agent's emoji or initials centered) as a data URL. We hand vis-network
// this instead of `shape: 'dot'` so the node visual IS the agent's identity
// glyph rather than a generic colored circle. Emoji render via the system font
// when available; chromium-edge delivers consistent emoji across desktop OSes.
export function agentAvatarDataUrl(emoji: string, color: string, size = 96): string {
  // Escape only what XML/URI needs: `&` to `&amp;`, `"` to `&quot;`. Browsers
  // also tolerate raw emoji bytes inside the SVG; encodeURIComponent over the
  // whole string keeps data URLs transport-safe (commas, quotes, `#`, etc.).
  const safeColor = color.replace(/"/g, '')
  const safeGlyph = emoji.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  const svg =
    `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 ${size} ${size}">` +
    `<defs><linearGradient id="g" x1="0%" y1="0%" x2="100%" y2="100%">` +
    `<stop offset="0%" stop-color="${safeColor}"/>` +
    `<stop offset="100%" stop-color="${safeColor}" stop-opacity="0.72"/>` +
    `</linearGradient></defs>` +
    `<circle cx="${size / 2}" cy="${size / 2}" r="${size / 2}" fill="url(#g)" ` +
    `stroke="${safeColor}" stroke-width="2"/>` +
    `<text x="50%" y="50%" text-anchor="middle" dominant-baseline="central" ` +
    `font-size="${size * 0.5}" fill="${avatarForeground(safeColor)}" font-family="system-ui, -apple-system, 'Segoe UI Emoji', 'Noto Color Emoji', sans-serif" ` +
    `font-weight="600">${safeGlyph}</text>` +
    `</svg>`
  return 'data:image/svg+xml;utf8,' + encodeURIComponent(svg)
}
