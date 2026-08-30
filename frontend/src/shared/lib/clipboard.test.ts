import { afterEach, describe, expect, it, vi } from 'vitest'
import { MAX_SOURCE_BYTES } from '@/features/image-annotator/imageAnnotatorLimits'
import { validateClipboardImage } from './clipboard'

const signatures = {
  'image/png': [137, 80, 78, 71, 13, 10, 26, 10],
  'image/jpeg': [0xff, 0xd8, 0xff],
  'image/webp': [82, 73, 70, 70, 0, 0, 0, 0, 87, 69, 66, 80],
} as const

function imageFile(type: keyof typeof signatures, name = 'paste.bin') {
  return new File([new Uint8Array(signatures[type])], name, { type })
}

function mockBitmap(width = 10, height = 10) {
  const close = vi.fn()
  vi.stubGlobal('createImageBitmap', vi.fn().mockResolvedValue({ width, height, close }))
  return close
}

afterEach(() => vi.unstubAllGlobals())

describe('validateClipboardImage', () => {
  it.each(Object.keys(signatures))(
    'accepts supported MIME and matching header: %s',
    async (type) => {
      const close = mockBitmap()
      const result = await validateClipboardImage(imageFile(type as keyof typeof signatures))
      result.dispose()
      result.dispose()
      expect(close).toHaveBeenCalledTimes(1)
    },
  )

  it.each([
    [
      new File([new Uint8Array(signatures['image/png'])], 'x', { type: '' }),
      'IMAGE_UNSUPPORTED_TYPE',
    ],
    [
      new File([new Uint8Array(signatures['image/png'])], 'x', { type: 'image/gif' }),
      'IMAGE_UNSUPPORTED_TYPE',
    ],
    [
      new File([new Uint8Array(signatures['image/jpeg'])], 'x', { type: 'image/png' }),
      'IMAGE_HEADER_INVALID',
    ],
  ])('rejects MIME/header failures before decode', async (file, code) => {
    const decode = vi.fn()
    vi.stubGlobal('createImageBitmap', decode)
    await expect(validateClipboardImage(file)).rejects.toMatchObject({ code })
    expect(decode).not.toHaveBeenCalled()
  })

  it('maps decoder rejection and non-positive dimensions to IMAGE_DECODE_FAILED', async () => {
    vi.stubGlobal('createImageBitmap', vi.fn().mockRejectedValue(new Error('broken')))
    await expect(validateClipboardImage(imageFile('image/png'))).rejects.toMatchObject({
      code: 'IMAGE_DECODE_FAILED',
    })
    const close = mockBitmap(0, 1)
    await expect(validateClipboardImage(imageFile('image/png'))).rejects.toMatchObject({
      code: 'IMAGE_DECODE_FAILED',
    })
    expect(close).toHaveBeenCalledOnce()
  })

  it.each([
    [8193, 1, 'IMAGE_DIMENSION_EXCEEDED'],
    [8000, 5001, 'IMAGE_PIXEL_LIMIT_EXCEEDED'],
  ])('rejects source limits and closes decoded bitmap', async (width, height, code) => {
    const close = mockBitmap(width, height)
    await expect(validateClipboardImage(imageFile('image/png'))).rejects.toMatchObject({ code })
    expect(close).toHaveBeenCalledOnce()
  })

  it('rejects declared and streamed byte overflow without decoding', async () => {
    const decode = vi.fn()
    vi.stubGlobal('createImageBitmap', decode)
    const declared = new File([new Uint8Array(MAX_SOURCE_BYTES + 1)], 'large.png', {
      type: 'image/png',
    })
    await expect(validateClipboardImage(declared)).rejects.toMatchObject({
      code: 'IMAGE_TOO_LARGE_BYTES',
    })

    const cancel = vi.fn()
    const reader = {
      read: vi
        .fn()
        .mockResolvedValueOnce({ done: false, value: new Uint8Array(MAX_SOURCE_BYTES) })
        .mockResolvedValueOnce({ done: false, value: new Uint8Array(1) }),
      cancel,
      releaseLock: vi.fn(),
    }
    const lyingFile = {
      size: 1,
      type: 'image/png',
      stream: () => ({ getReader: () => reader }),
    } as unknown as File
    await expect(validateClipboardImage(lyingFile)).rejects.toMatchObject({
      code: 'IMAGE_TOO_LARGE_BYTES',
    })
    expect(cancel).toHaveBeenCalledOnce()
    expect(decode).not.toHaveBeenCalled()
  })
})
