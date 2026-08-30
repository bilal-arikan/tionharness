// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { artifactApi } from '@/api/artifacts'
import type { Artifact } from '@/types'
import { MAX_SOURCE_BYTES } from '@/features/image-annotator/imageAnnotatorLimits'
import { readImageResponse } from './imageArtifactSource'

const apiMock = vi.hoisted(() => ({
  getArtifact: vi.fn(),
  getArtifactSource: vi.fn(),
  uploadFile: vi.fn(),
  createArtifact: vi.fn(),
}))

vi.mock('@/api', () => ({ api: apiMock }))

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
      new Response(new Uint8Array([0x89, 0x50, 0x4e, 0x47]), {
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
      new Response(new Uint8Array([0x89, 0x50, 0x4e, 0x47]), {
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
})
