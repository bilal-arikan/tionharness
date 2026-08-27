import { describe, expect, it } from 'vitest'
import { THEME_PRESETS } from './themePresets'

function relativeLuminance(hex: string): number {
  const channels = hex
    .slice(1)
    .match(/.{2}/g)!
    .map((channel) => Number.parseInt(channel, 16) / 255)
    .map((channel) => (channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4))

  return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2]
}

function contrastRatio(a: string, b: string): number {
  const lighter = Math.max(relativeLuminance(a), relativeLuminance(b))
  const darker = Math.min(relativeLuminance(a), relativeLuminance(b))
  return (lighter + 0.05) / (darker + 0.05)
}

describe('sender bubble theme tokens', () => {
  it('defines complete sender bubble colors for every preset and mode', () => {
    expect(THEME_PRESETS.some((preset) => preset.dark)).toBe(true)
    expect(THEME_PRESETS.some((preset) => !preset.dark)).toBe(true)

    for (const preset of THEME_PRESETS) {
      expect(preset.tokens.senderBubble, preset.id).toMatch(/^#[0-9a-f]{6}$/i)
      expect(preset.tokens.onSenderBubble, preset.id).toMatch(/^#[0-9a-f]{6}$/i)
      expect(preset.tokens.senderBubbleBorder, preset.id).toMatch(/^#[0-9a-f]{6}$/i)
    }
  })

  it('keeps sender bubble body text at WCAG AA contrast', () => {
    for (const preset of THEME_PRESETS) {
      expect(
        contrastRatio(preset.tokens.senderBubble, preset.tokens.onSenderBubble),
        preset.id,
      ).toBeGreaterThanOrEqual(4.5)
    }
  })

  it('keeps the sender bubble border distinguishable from its fill', () => {
    for (const preset of THEME_PRESETS) {
      expect(
        contrastRatio(preset.tokens.senderBubble, preset.tokens.senderBubbleBorder),
        preset.id,
      ).toBeGreaterThan(1.2)
    }
  })
})
