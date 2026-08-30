import { describe, expect, it } from 'vitest'
import {
  ImageAnnotatorError,
  MAX_SOURCE_BYTES,
  validateImageSource,
  validateImageHeader,
  validateSourceLimits,
} from './imageAnnotatorLimits'

const codeOf = (run: () => void) => {
  try {
    run()
    return null
  } catch (error) {
    return (error as ImageAnnotatorError).code
  }
}

describe('image source limits', () => {
  it.each([MAX_SOURCE_BYTES - 1, MAX_SOURCE_BYTES])('accepts byte count %i', (size) =>
    expect(() => validateSourceLimits(size, 1, 1)).not.toThrow(),
  )
  it('rejects first byte over limit', () =>
    expect(codeOf(() => validateSourceLimits(MAX_SOURCE_BYTES + 1, 1, 1))).toBe(
      'IMAGE_TOO_LARGE_BYTES',
    ))
  it.each([8191, 8192])('accepts dimension %i', (width) =>
    expect(() => validateSourceLimits(1, width, 1)).not.toThrow(),
  )
  it('rejects dimension 8193', () =>
    expect(codeOf(() => validateSourceLimits(1, 8193, 1))).toBe('IMAGE_DIMENSION_EXCEEDED'))
  it.each([39_999_999, 40_000_000])('accepts pixel count %i', (pixels) =>
    expect(() => validateSourceLimits(1, 5000, pixels / 5000)).not.toThrow(),
  )
  it('rejects 40,000,001 pixels', () =>
    expect(codeOf(() => validateSourceLimits(1, 5000, 40_000_001 / 5000))).toBe(
      'IMAGE_PIXEL_LIMIT_EXCEEDED',
    ))
  it('checks decode before dimensions', () =>
    expect(codeOf(() => validateSourceLimits(1, 0, 9000))).toBe('IMAGE_DECODE_FAILED'))
})

describe('image headers', () => {
  const png = new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10])
  const jpeg = new Uint8Array([0xff, 0xd8, 0xff, 0xe0])
  const webp = new TextEncoder().encode('RIFFxxxxWEBP')
  it.each([
    ['image/png', png],
    ['image/jpeg', jpeg],
    ['image/webp', webp],
  ] as const)('accepts %s', (mime, bytes) =>
    expect(() => validateImageHeader(mime, bytes)).not.toThrow(),
  )
  it.each(['', 'image/gif'])('rejects unsupported MIME %j', (mime) =>
    expect(codeOf(() => validateImageHeader(mime, png))).toBe('IMAGE_UNSUPPORTED_TYPE'),
  )
  it('rejects MIME/header mismatch', () =>
    expect(codeOf(() => validateImageHeader('image/jpeg', png))).toBe('IMAGE_HEADER_INVALID'))
  it('rejects declared and actual oversized input before header/decode', () => {
    expect(codeOf(() => validateImageSource('image/png', png, 1, 1, MAX_SOURCE_BYTES + 1))).toBe(
      'IMAGE_TOO_LARGE_BYTES',
    )
    const oversized = new Uint8Array(MAX_SOURCE_BYTES + 1)
    expect(codeOf(() => validateImageSource('image/png', oversized, 1, 1, 1))).toBe(
      'IMAGE_TOO_LARGE_BYTES',
    )
  })
})
