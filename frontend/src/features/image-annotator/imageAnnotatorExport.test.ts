// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from 'vitest'
import { exportAnnotation, type DrawableSource } from './imageAnnotatorExport'

const context = {
  lineCap: '',
  lineJoin: '',
  beginPath: vi.fn(),
  moveTo: vi.fn(),
  lineTo: vi.fn(),
  stroke: vi.fn(),
  strokeStyle: '',
  lineWidth: 0,
} as unknown as CanvasRenderingContext2D
const source = (opaque: boolean): DrawableSource => ({
  width: 10,
  height: 20,
  opaque,
  draw: vi.fn(),
})

afterEach(() => vi.restoreAllMocks())

describe('annotation export', () => {
  it('keeps transparent output as PNG', async () => {
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(context)
    vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation((callback, mime) =>
      callback(new Blob(['x'], { type: mime })),
    )
    const result = await exportAnnotation(source(false), [])
    expect(result.mime).toBe('image/png')
  })

  it('uses WebP for opaque output when browser returns actual WebP', async () => {
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(context)
    vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation((callback, mime) =>
      callback(new Blob(['x'], { type: mime })),
    )
    expect((await exportAnnotation(source(true), [])).mime).toBe('image/webp')
  })

  it('falls back exactly once to PNG for null or mismatched WebP', async () => {
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(context)
    const encode = vi
      .spyOn(HTMLCanvasElement.prototype, 'toBlob')
      .mockImplementationOnce((callback) => callback(new Blob(['x'], { type: 'image/png' })))
      .mockImplementationOnce((callback) => callback(new Blob(['x'], { type: 'image/png' })))
    expect((await exportAnnotation(source(true), [])).mime).toBe('image/png')
    expect(encode).toHaveBeenCalledTimes(2)
  })

  it('surfaces stable error after failed fallback', async () => {
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(context)
    vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation((callback) => callback(null))
    await expect(exportAnnotation(source(true), [])).rejects.toMatchObject({
      code: 'EXPORT_FAILED',
    })
  })

  it('releases the export backing store after encoding', async () => {
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(context)
    vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation((callback, mime) =>
      callback(new Blob(['x'], { type: mime })),
    )
    const createElement = document.createElement.bind(document)
    const exportCanvases: HTMLCanvasElement[] = []
    vi.spyOn(document, 'createElement').mockImplementation((tagName, options) => {
      const element = createElement(tagName, options)
      if (tagName === 'canvas') exportCanvases.push(element as HTMLCanvasElement)
      return element
    })

    await exportAnnotation(source(false), [])

    expect(exportCanvases.at(-1)?.width).toBe(0)
    expect(exportCanvases.at(-1)?.height).toBe(0)
  })
})
