// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { artifactApi } from '@/api/artifacts'
import type { Artifact } from '@/types'
import { MAX_SOURCE_BYTES } from '@/features/image-annotator/imageAnnotatorLimits'
import { readImageResponse } from './imageArtifactSource'
import type {
  AnnotatorExport,
  DrawableSource,
} from '@/features/image-annotator/imageAnnotatorExport'
import { i18next } from '@/i18n'

const apiMock = vi.hoisted(() => ({
  getArtifact: vi.fn(),
  getArtifactSource: vi.fn(),
  uploadFile: vi.fn(),
  createArtifact: vi.fn(),
}))

let annotatorProps: null | {
  source: DrawableSource
  onSave(result: AnnotatorExport): Promise<void>
  onClose(): void
} = null

vi.mock('@/api', () => ({ api: apiMock }))
vi.mock('@/features/image-annotator/ImageAnnotator', () => ({
  ImageAnnotator: (props: NonNullable<typeof annotatorProps>) => {
    annotatorProps = props
    return <div data-testid="image-annotator" />
  },
}))

import { ArtifactPreviewModal } from './ArtifactPreviewModal'

const imageArtifact: Artifact = {
  id: 'ART1',
  sessionId: 'SES1',
  agentId: '',
  title: 'Kaynak',
  kind: 'image',
  language: '',
  content: '',
  sourcePath: 'uploads/source.png',
  origin: 'manual',
  createdAt: 1,
  updatedAt: 1,
}

const pngHeader = new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => {
    resolve = done
  })
  return { promise, resolve }
}

describe('artifact-origin image source', () => {
  let container: HTMLDivElement
  let root: Root | null

  beforeEach(() => {
    void i18next.changeLanguage('tr')
    vi.clearAllMocks()
    annotatorProps = null
    container = document.createElement('div')
    document.body.appendChild(container)
    root = null
    ;(
      globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true
    apiMock.getArtifact.mockResolvedValue(imageArtifact)
  })

  afterEach(async () => {
    if (root) await act(async () => root?.unmount())
    container.remove()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('rejects declared oversized source before reading body', async () => {
    const response = new Response(new ReadableStream(), {
      headers: { 'Content-Length': String(MAX_SOURCE_BYTES + 1) },
    })
    await expect(readImageResponse(response)).rejects.toThrow('Görsel 20 MB sınırını aşıyor.')
  })

  it('cancels at first streamed byte beyond the limit', async () => {
    const cancel = vi.fn()
    const response = new Response(
      new ReadableStream({
        start(controller) {
          controller.enqueue(new Uint8Array(MAX_SOURCE_BYTES))
          controller.enqueue(new Uint8Array([1]))
        },
        cancel,
      }),
    )
    await expect(readImageResponse(response)).rejects.toThrow('Görsel 20 MB sınırını aşıyor.')
    expect(cancel).toHaveBeenCalledOnce()
  })

  it.each([401, 403])(
    'does not automatically retry source permission status %i',
    async (status) => {
      const fetchMock = vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: 'izin yok' }), {
          status,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
      vi.stubGlobal('fetch', fetchMock)
      await expect(artifactApi.getArtifactSource('ART1')).rejects.toThrow('izin yok')
      expect(fetchMock).toHaveBeenCalledOnce()
    },
  )

  it('closes a bitmap that resolves after unmount without reporting a late error', async () => {
    const bitmapResult = deferred<ImageBitmap>()
    const close = vi.fn()
    const onError = vi.fn()
    apiMock.getArtifactSource.mockResolvedValue(
      new Response(pngHeader, {
        headers: { 'Content-Type': 'image/png' },
      }),
    )
    vi.stubGlobal(
      'createImageBitmap',
      vi.fn(() => bitmapResult.promise),
    )
    root = createRoot(container)

    await act(async () => {
      root?.render(<ArtifactPreviewModal artifactId="ART1" onClose={vi.fn()} onError={onError} />)
    })
    await act(async () => {})
    const editButton = container.querySelector<HTMLButtonElement>(
      'button[title="Orijinali koruyarak üzerine çiz"]',
    )
    expect(editButton).not.toBeNull()
    await act(async () => editButton?.click())
    await act(async () => {
      root?.unmount()
      root = null
    })
    const signal = apiMock.getArtifactSource.mock.calls[0][1] as AbortSignal
    expect(signal.aborted).toBe(true)
    bitmapResult.resolve({ width: 1, height: 1, close } as ImageBitmap)
    await act(async () => bitmapResult.promise)

    expect(close).toHaveBeenCalledOnce()
    expect(onError).not.toHaveBeenCalled()
  })

  it('closes the owned bitmap exactly once when unmounted after opening', async () => {
    const close = vi.fn()
    apiMock.getArtifactSource.mockResolvedValue(
      new Response(pngHeader, {
        headers: { 'Content-Type': 'image/png' },
      }),
    )
    vi.stubGlobal(
      'createImageBitmap',
      vi.fn().mockResolvedValue({ width: 1, height: 1, close } as ImageBitmap),
    )
    root = createRoot(container)

    await act(async () => {
      root?.render(<ArtifactPreviewModal artifactId="ART1" onClose={vi.fn()} />)
    })
    await act(async () => {})
    await act(async () =>
      container
        .querySelector<HTMLButtonElement>('button[title="Orijinali koruyarak üzerine çiz"]')
        ?.click(),
    )
    await act(async () => {
      root?.unmount()
      root = null
    })

    expect(close).toHaveBeenCalledOnce()
  })

  it('creates and reveals a derived artifact without updating the original', async () => {
    const close = vi.fn()
    const onOpenFull = vi.fn()
    apiMock.getArtifactSource.mockResolvedValue(
      new Response(pngHeader, {
        headers: { 'Content-Type': 'image/png' },
      }),
    )
    vi.stubGlobal(
      'createImageBitmap',
      vi.fn().mockResolvedValue({ width: 1, height: 1, close } as ImageBitmap),
    )
    apiMock.uploadFile.mockResolvedValue({ relPath: 'artifacts/SES1/staged.png' })
    apiMock.createArtifact.mockResolvedValue({ ...imageArtifact, id: 'ART2' })
    root = createRoot(container)

    await act(async () => {
      root?.render(
        <ArtifactPreviewModal artifactId="ART1" onClose={vi.fn()} onOpenFull={onOpenFull} />,
      )
    })
    await act(async () => {})
    await act(async () =>
      container
        .querySelector<HTMLButtonElement>('button[title="Orijinali koruyarak üzerine çiz"]')
        ?.click(),
    )
    await act(async () => {})
    expect(annotatorProps).not.toBeNull()
    await act(async () =>
      annotatorProps?.onSave({
        blob: new Blob(['png'], { type: 'image/png' }),
        mime: 'image/png',
        width: 1,
        height: 1,
      }),
    )

    expect(apiMock.createArtifact).toHaveBeenCalledWith({
      title: 'Kaynak — Düzenleme',
      kind: 'image',
      sessionId: 'SES1',
      sourcePath: 'artifacts/SES1/staged.png',
      origin: 'manual',
      derivedFromArtifactId: 'ART1',
    })
    expect(apiMock.getArtifact).toHaveBeenCalledWith('ART1')
    expect(apiMock.getArtifact).toHaveBeenCalledTimes(1)
    expect(close).toHaveBeenCalledOnce()
    expect(onOpenFull).toHaveBeenCalledWith('ART2')
  })

  it('keeps the editor open and rejects when derived artifact persistence fails', async () => {
    const close = vi.fn()
    const failure = new Error('DERIVED_ARTIFACT_SAVE_FAILED')
    apiMock.getArtifactSource.mockResolvedValue(
      new Response(pngHeader, {
        headers: { 'Content-Type': 'image/png' },
      }),
    )
    vi.stubGlobal(
      'createImageBitmap',
      vi.fn().mockResolvedValue({ width: 1, height: 1, close } as ImageBitmap),
    )
    apiMock.uploadFile.mockResolvedValue({ relPath: 'artifacts/SES1/staged.png' })
    apiMock.createArtifact.mockRejectedValue(failure)
    root = createRoot(container)

    await act(async () => {
      root?.render(<ArtifactPreviewModal artifactId="ART1" onClose={vi.fn()} />)
    })
    await act(async () => {})
    await act(async () =>
      container
        .querySelector<HTMLButtonElement>('button[title="Orijinali koruyarak üzerine çiz"]')
        ?.click(),
    )
    await act(async () => {})
    expect(annotatorProps).not.toBeNull()

    await expect(
      annotatorProps?.onSave({
        blob: new Blob(['png'], { type: 'image/png' }),
        mime: 'image/png',
        width: 1,
        height: 1,
      }),
    ).rejects.toThrow('DERIVED_ARTIFACT_SAVE_FAILED')
    expect(container.querySelector('[data-testid="image-annotator"]')).not.toBeNull()
    expect(close).not.toHaveBeenCalled()
  })
})
