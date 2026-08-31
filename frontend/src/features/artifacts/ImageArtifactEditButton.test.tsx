// @vitest-environment jsdom

import { act, useState } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Artifact } from '@/types'
import type {
  AnnotatorExport,
  DrawableSource,
} from '@/features/image-annotator/imageAnnotatorExport'
import { i18next } from '@/i18n'

const apiMock = vi.hoisted(() => ({
  getArtifactSource: vi.fn(),
  uploadFile: vi.fn(),
  updateArtifact: vi.fn(),
  createArtifact: vi.fn(),
  deleteFile: vi.fn(),
}))

let annotatorProps: null | {
  source: DrawableSource
  onSave(result: AnnotatorExport): Promise<void>
  onClose(): void
} = null

vi.mock('@/api', () => ({ api: apiMock }))
// The real annotator needs a canvas; this stand-in keeps the same contract the
// component depends on (source in, onSave/onClose out) and renders whatever the
// save rejects with, which is how the user sees a failure.
vi.mock('@/features/image-annotator/ImageAnnotator', () => ({
  ImageAnnotator: (props: NonNullable<typeof annotatorProps>) => {
    const [error, setError] = useState<string | null>(null)
    annotatorProps = {
      ...props,
      onSave: async (result) => {
        try {
          await props.onSave(result)
        } catch (reason) {
          setError(reason instanceof Error ? reason.message : String(reason))
          throw reason
        }
      },
    }
    return <div data-testid="image-annotator">{error && <div role="alert">{error}</div>}</div>
  },
}))

import { ImageArtifactEditButton } from './ImageArtifactEditButton'

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

const exported: AnnotatorExport = {
  blob: new Blob(['png'], { type: 'image/png' }),
  mime: 'image/png',
  width: 1,
  height: 1,
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => {
    resolve = done
  })
  return { promise, resolve }
}

function stubDecodableSource(close = vi.fn()) {
  apiMock.getArtifactSource.mockResolvedValue(
    new Response(pngHeader, { headers: { 'Content-Type': 'image/png' } }),
  )
  vi.stubGlobal(
    'createImageBitmap',
    vi.fn().mockResolvedValue({ width: 1, height: 1, close } as ImageBitmap),
  )
  return close
}

describe('ImageArtifactEditButton', () => {
  let container: HTMLDivElement
  let root: Root | null
  let onUpdated: ReturnType<typeof vi.fn>
  let onError: ReturnType<typeof vi.fn>

  const editButton = () =>
    container.querySelector<HTMLButtonElement>('[data-testid="artifact-image-edit"]')

  const render = async (artifact: Artifact = imageArtifact) => {
    root = createRoot(container)
    await act(async () => {
      root?.render(
        <ImageArtifactEditButton artifact={artifact} onUpdated={onUpdated} onError={onError} />,
      )
    })
  }

  const openAnnotator = async () => {
    await act(async () => editButton()?.click())
    await act(async () => {})
  }

  // The stand-in annotator re-throws, and the rejection also flips its own state,
  // so both have to be awaited inside act().
  const save = async () => {
    let rejected: unknown = null
    await act(async () => {
      try {
        await annotatorProps?.onSave(exported)
      } catch (reason) {
        rejected = reason
      }
    })
    return rejected
  }

  beforeEach(() => {
    void i18next.changeLanguage('tr')
    vi.clearAllMocks()
    annotatorProps = null
    container = document.createElement('div')
    document.body.appendChild(container)
    root = null
    onUpdated = vi.fn()
    onError = vi.fn()
    ;(
      globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true
  })

  afterEach(async () => {
    if (root) await act(async () => root?.unmount())
    container.remove()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('renders nothing for a non-image artifact', async () => {
    await render({ ...imageArtifact, kind: 'text', content: 'merhaba', sourcePath: '' })

    expect(editButton()).toBeNull()
    expect(container.textContent).toBe('')
  })

  it('renders nothing for an image artifact without a source file', async () => {
    await render({ ...imageArtifact, sourcePath: '' })

    expect(editButton()).toBeNull()
  })

  it('loads the source and opens the annotator on click', async () => {
    stubDecodableSource()
    await render()
    await openAnnotator()

    expect(apiMock.getArtifactSource).toHaveBeenCalledOnce()
    expect(apiMock.getArtifactSource.mock.calls[0][0]).toBe('ART1')
    expect(container.querySelector('[data-testid="image-annotator"]')).not.toBeNull()
    expect(annotatorProps?.source.width).toBe(1)
  })

  it('reports a source load failure without opening the annotator', async () => {
    apiMock.getArtifactSource.mockRejectedValue(new Error('KAYNAK_OKUNAMADI'))
    await render()
    await openAnnotator()

    expect(onError).toHaveBeenCalledWith('KAYNAK_OKUNAMADI')
    expect(container.querySelector('[data-testid="image-annotator"]')).toBeNull()
    expect(editButton()?.disabled).toBe(false)
  })

  it('disables the button while the source is still loading', async () => {
    const pending = deferred<Response>()
    apiMock.getArtifactSource.mockReturnValue(pending.promise)
    await render()
    await act(async () => editButton()?.click())

    expect(editButton()?.disabled).toBe(true)

    pending.resolve(new Response(pngHeader, { headers: { 'Content-Type': 'image/png' } }))
    vi.stubGlobal(
      'createImageBitmap',
      vi.fn().mockResolvedValue({ width: 1, height: 1, close: vi.fn() } as ImageBitmap),
    )
    await act(async () => pending.promise)
  })

  it('overwrites the same artifact instead of creating a derivative', async () => {
    const close = stubDecodableSource()
    const updated: Artifact = {
      ...imageArtifact,
      sourcePath: 'artifacts/SES1/ART1-edited.png',
      updatedAt: 2,
    }
    apiMock.uploadFile.mockResolvedValue({ relPath: 'artifacts/SES1/ART1-edited.png' })
    apiMock.updateArtifact.mockResolvedValue(updated)
    await render()
    await openAnnotator()

    expect(await save()).toBeNull()

    expect(apiMock.uploadFile).toHaveBeenCalledOnce()
    expect(apiMock.uploadFile.mock.calls[0][0]).toBe('SES1')
    const uploaded = apiMock.uploadFile.mock.calls[0][1] as File
    expect(uploaded.name).toBe('ART1-edited.png')
    expect(apiMock.updateArtifact).toHaveBeenCalledOnce()
    expect(apiMock.updateArtifact).toHaveBeenCalledWith('ART1', {
      sourcePath: 'artifacts/SES1/ART1-edited.png',
    })
    expect(apiMock.createArtifact).not.toHaveBeenCalled()
    expect(apiMock.deleteFile).not.toHaveBeenCalled()
    expect(onUpdated).toHaveBeenCalledWith(updated)
    expect(close).toHaveBeenCalledOnce()
    expect(container.querySelector('[data-testid="image-annotator"]')).toBeNull()
  })

  it('surfaces a missing staging path and never issues the update', async () => {
    stubDecodableSource()
    apiMock.uploadFile.mockResolvedValue({ relPath: '' })
    await render()
    await openAnnotator()

    const rejected = await save()

    expect(rejected).toBeInstanceOf(Error)
    expect(apiMock.updateArtifact).not.toHaveBeenCalled()
    expect(onUpdated).not.toHaveBeenCalled()
    expect(container.querySelector('[role="alert"]')?.textContent).toBe(
      i18next.t('artifactAnnotation.stagingPathError', { ns: 'common' }),
    )
  })

  it('removes the staged upload and keeps the artifact when the update fails', async () => {
    const close = stubDecodableSource()
    const failure = new Error('ARTIFACT_UPDATE_FAILED')
    apiMock.uploadFile.mockResolvedValue({ relPath: 'artifacts/SES1/ART1-edited.png' })
    apiMock.updateArtifact.mockRejectedValue(failure)
    apiMock.deleteFile.mockResolvedValue(undefined)
    await render()
    await openAnnotator()

    expect(await save()).toBe(failure)

    expect(apiMock.deleteFile).toHaveBeenCalledWith('artifacts/SES1/ART1-edited.png')
    expect(onUpdated).not.toHaveBeenCalled()
    expect(close).not.toHaveBeenCalled()
    expect(container.querySelector('[data-testid="image-annotator"]')).not.toBeNull()
    expect(container.querySelector('[role="alert"]')?.textContent).toBe('ARTIFACT_UPDATE_FAILED')
  })

  it('reports both failures when the staged upload cannot be cleaned up either', async () => {
    stubDecodableSource()
    const updateFailure = new Error('ARTIFACT_UPDATE_FAILED')
    const cleanupFailure = new Error('STAGED_UPLOAD_CLEANUP_FAILED')
    apiMock.uploadFile.mockResolvedValue({ relPath: 'artifacts/SES1/ART1-edited.png' })
    apiMock.updateArtifact.mockRejectedValue(updateFailure)
    apiMock.deleteFile.mockRejectedValue(cleanupFailure)
    await render()
    await openAnnotator()

    const rejected = await save()

    expect(rejected).toBeInstanceOf(AggregateError)
    expect((rejected as AggregateError).errors).toEqual([updateFailure, cleanupFailure])
    expect((rejected as AggregateError).cause).toBe(cleanupFailure)
    expect(apiMock.deleteFile).toHaveBeenCalledWith('artifacts/SES1/ART1-edited.png')
    expect(onUpdated).not.toHaveBeenCalled()
    expect(container.querySelector('[role="alert"]')?.textContent).toBe(
      i18next.t('artifactAnnotation.overwriteAndCleanupError', { ns: 'common' }),
    )
  })
})
