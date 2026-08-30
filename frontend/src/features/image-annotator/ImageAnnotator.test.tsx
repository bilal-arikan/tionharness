// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ImageAnnotator } from './ImageAnnotator'
import { i18next } from '@/i18n'

const context = {
  clearRect: vi.fn(),
  save: vi.fn(),
  restore: vi.fn(),
  scale: vi.fn(),
  beginPath: vi.fn(),
  moveTo: vi.fn(),
  lineTo: vi.fn(),
  stroke: vi.fn(),
  lineCap: '',
  lineJoin: '',
  strokeStyle: '',
  lineWidth: 0,
} as unknown as CanvasRenderingContext2D

class ResizeObserverMock {
  observe() {}
  disconnect() {}
}

let host: HTMLDivElement
let root: Root
let devicePixelRatio = 1
let dprChangeListener: (() => void) | undefined
let renderedWidths: number[]

beforeEach(() => {
  void i18next.changeLanguage('tr')
  devicePixelRatio = 1
  dprChangeListener = undefined
  renderedWidths = []
  source.draw.mockClear()
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  host = document.createElement('div')
  document.body.append(host)
  root = createRoot(host)
  vi.stubGlobal('ResizeObserver', ResizeObserverMock)
  vi.spyOn(window, 'devicePixelRatio', 'get').mockImplementation(() => devicePixelRatio)
  vi.stubGlobal(
    'matchMedia',
    vi.fn().mockImplementation(() => ({
      matches: true,
      media: '',
      onchange: null,
      addEventListener: (_type: string, listener: () => void) => {
        dprChangeListener = listener
      },
      removeEventListener: (_type: string, listener: () => void) => {
        if (dprChangeListener === listener) dprChangeListener = undefined
      },
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  )
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(context)
  Object.defineProperty(context, 'lineWidth', {
    configurable: true,
    get: () => renderedWidths.at(-1) ?? 0,
    set: (value: number) => renderedWidths.push(value),
  })
  vi.spyOn(HTMLCanvasElement.prototype, 'getBoundingClientRect').mockReturnValue({
    x: 0,
    y: 0,
    top: 0,
    left: 0,
    right: 100,
    bottom: 100,
    width: 100,
    height: 100,
    toJSON: () => ({}),
  })
  HTMLCanvasElement.prototype.setPointerCapture = vi.fn()
})

afterEach(() => {
  act(() => root.unmount())
  host.remove()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

const source = { width: 100, height: 100, opaque: false, draw: vi.fn() }

function pointer(target: Element, type: string, pointerId = 1) {
  const event = new MouseEvent(type, { bubbles: true, button: 0, clientX: 20, clientY: 30 })
  Object.defineProperties(event, {
    isPrimary: { value: true },
    pointerId: { value: pointerId },
    pressure: { value: 0.5 },
  })
  target.dispatchEvent(event)
}

describe('ImageAnnotator', () => {
  it('updates the backing store and redraws when only DPR changes', () => {
    act(() => root.render(<ImageAnnotator source={source} onSave={vi.fn()} onClose={vi.fn()} />))
    const canvas = host.querySelector('canvas')!
    expect(canvas.width).toBe(100)
    expect(canvas.height).toBe(100)
    const drawsBeforeDprChange = source.draw.mock.calls.length

    devicePixelRatio = 2
    act(() => dprChangeListener?.())

    expect(canvas.width).toBe(200)
    expect(canvas.height).toBe(200)
    expect(source.draw).toHaveBeenCalledTimes(drawsBeforeDprChange + 1)
  })

  it('draws with primary pointer and enables undo/save', () => {
    act(() => root.render(<ImageAnnotator source={source} onSave={vi.fn()} onClose={vi.fn()} />))
    const canvas = host.querySelector('canvas')!
    act(() => {
      pointer(canvas, 'pointerdown')
      pointer(canvas, 'pointerup')
    })
    expect(host.querySelector<HTMLButtonElement>('[aria-label="Geri al"]')?.disabled).toBe(false)
    expect(host.querySelector<HTMLButtonElement>('[aria-label="Görseli kaydet"]')?.disabled).toBe(
      false,
    )
  })

  it('uses accessible selected pen size only for new strokes', () => {
    act(() => root.render(<ImageAnnotator source={source} onSave={vi.fn()} onClose={vi.fn()} />))
    const canvas = host.querySelector('canvas')!
    const size = host.querySelector<HTMLInputElement>('[aria-label="Kalem boyutu"]')!
    expect(size.value).toBe('2')

    act(() => {
      pointer(canvas, 'pointerdown')
      pointer(canvas, 'pointerup')
    })
    act(() => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(size, '6')
      size.dispatchEvent(new Event('input', { bubbles: true }))
    })
    expect(size.value).toBe('6')
    act(() => {
      pointer(canvas, 'pointerdown')
      pointer(canvas, 'pointerup')
    })

    expect(renderedWidths).toContain(2)
    expect(renderedWidths).toContain(6)
  })

  it('keeps only the drawing viewport transparent', () => {
    act(() => root.render(<ImageAnnotator source={source} onSave={vi.fn()} onClose={vi.fn()} />))
    expect(host.querySelector('[data-testid="drawing-viewport"]')?.className).toContain(
      'bg-transparent',
    )
    expect(host.querySelector('[role="dialog"]')?.className).toContain('bg-[var(--bg-primary)]')
  })

  it('ignores secondary pointers', () => {
    act(() => root.render(<ImageAnnotator source={source} onSave={vi.fn()} onClose={vi.fn()} />))
    const event = new MouseEvent('pointerdown', { bubbles: true, button: 0 })
    Object.defineProperties(event, { isPrimary: { value: false }, pointerId: { value: 2 } })
    act(() => host.querySelector('canvas')!.dispatchEvent(event))
    expect(host.querySelector<HTMLButtonElement>('[aria-label="Geri al"]')?.disabled).toBe(true)
  })

  it('undoes with Ctrl+Z and returns to clean baseline', () => {
    act(() => root.render(<ImageAnnotator source={source} onSave={vi.fn()} onClose={vi.fn()} />))
    const canvas = host.querySelector('canvas')!
    act(() => {
      pointer(canvas, 'pointerdown')
      pointer(canvas, 'pointerup')
    })
    act(() => window.dispatchEvent(new KeyboardEvent('keydown', { key: 'z', ctrlKey: true })))
    expect(host.querySelector<HTMLButtonElement>('[aria-label="Görseli kaydet"]')?.disabled).toBe(
      true,
    )
  })

  it('asks before Escape closes dirty drawing', () => {
    const onClose = vi.fn()
    vi.spyOn(window, 'confirm').mockReturnValue(false)
    act(() => root.render(<ImageAnnotator source={source} onSave={vi.fn()} onClose={onClose} />))
    const canvas = host.querySelector('canvas')!
    act(() => {
      pointer(canvas, 'pointerdown')
      pointer(canvas, 'pointerup')
    })
    act(() => window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })))
    expect(onClose).not.toHaveBeenCalled()
    expect(window.confirm).toHaveBeenCalledOnce()
  })

  it('surfaces a derived artifact save failure and keeps the drawing dirty', async () => {
    const onSave = vi.fn().mockRejectedValue(new Error('DERIVED_ARTIFACT_SAVE_FAILED'))
    vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation((callback) => {
      callback(new Blob(['png'], { type: 'image/png' }))
    })
    await act(async () =>
      root.render(<ImageAnnotator source={source} onSave={onSave} onClose={vi.fn()} />),
    )
    const canvas = host.querySelector('canvas')!
    act(() => {
      pointer(canvas, 'pointerdown')
      pointer(canvas, 'pointerup')
    })

    await act(async () =>
      host.querySelector<HTMLButtonElement>('[aria-label="Görseli kaydet"]')?.click(),
    )

    expect(host.querySelector('[role="alert"]')?.textContent).toBe('DERIVED_ARTIFACT_SAVE_FAILED')
    expect(host.querySelector<HTMLButtonElement>('[aria-label="Görseli kaydet"]')?.disabled).toBe(
      false,
    )
  })
})
