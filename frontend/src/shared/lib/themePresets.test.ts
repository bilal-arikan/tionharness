import { describe, expect, it } from 'vitest'
import { THEME_COLORS, THEME_PRESETS } from './themePresets'

const HEX_COLOR = /^#[0-9a-f]{6}$/i

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
      expect(preset.tokens.senderBubble, preset.id).toMatch(HEX_COLOR)
      expect(preset.tokens.onSenderBubble, preset.id).toMatch(HEX_COLOR)
      expect(preset.tokens.senderBubbleBorder, preset.id).toMatch(HEX_COLOR)
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

describe('theme preset catalog', () => {
  it('uses unique preset and color family ids', () => {
    const presetIds = THEME_PRESETS.map((preset) => preset.id)
    const colorIds = THEME_COLORS.map((color) => color.id)

    expect(new Set(presetIds).size).toBe(presetIds.length)
    expect(new Set(colorIds).size).toBe(colorIds.length)
  })

  it('defines exactly one dark and one light preset for every color family', () => {
    for (const color of THEME_COLORS) {
      expect(THEME_PRESETS.filter((preset) => preset.id === color.dark.id)).toHaveLength(1)
      expect(THEME_PRESETS.find((preset) => preset.id === color.dark.id)?.dark).toBe(true)
      expect(THEME_PRESETS.filter((preset) => preset.id === color.light.id)).toHaveLength(1)
      expect(THEME_PRESETS.find((preset) => preset.id === color.light.id)?.dark).toBe(false)
    }

    expect(THEME_PRESETS).toHaveLength(THEME_COLORS.length * 2)
  })

  it('keeps THEME_COLORS variant ids synchronized with THEME_PRESETS', () => {
    const presetIds = new Set(THEME_PRESETS.map((preset) => preset.id))
    const colorVariantIds = new Set(
      THEME_COLORS.flatMap((color) => [color.dark.id, color.light.id]),
    )

    expect(colorVariantIds).toEqual(presetIds)
  })

  it('defines every theme token as a six-digit hex color', () => {
    for (const preset of THEME_PRESETS) {
      for (const [token, value] of Object.entries(preset.tokens)) {
        expect(value, `${preset.id}.${token}`).toMatch(HEX_COLOR)
      }
    }
  })

  it('keeps accent text at WCAG AA contrast', () => {
    for (const preset of THEME_PRESETS) {
      expect(
        contrastRatio(preset.tokens.accent, preset.tokens.onAccent),
        preset.id,
      ).toBeGreaterThanOrEqual(4.5)
    }
  })

  it('keeps danger text at WCAG AA contrast', () => {
    for (const preset of THEME_PRESETS) {
      expect(
        contrastRatio(preset.tokens.danger, preset.tokens.onDanger),
        preset.id,
      ).toBeGreaterThanOrEqual(4.5)
    }
  })
})
