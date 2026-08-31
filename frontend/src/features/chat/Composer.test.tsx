// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AnnotatorExport } from '@/features/image-annotator/imageAnnotatorExport'
import type { Agent } from '@/types'
import { Composer } from './Composer'

const mocks = vi.hoisted(() => ({
  uploadFile: vi.fn(),
  deleteFile: vi.fn(),
  annotatorProps: null as null | {
    source: unknown
    onSave(result: AnnotatorExport): Promise<void>
    onClose(): void
  },
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: { ...actual.api, uploadFile: mocks.uploadFile, deleteFile: mocks.deleteFile },
  }
})
vi.mock('@/shared/lib/catalog', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/shared/lib/catalog')>()
  return {
    ...actual,
    useCatalog: () => [],
    thinkingInfoForModel: () => ({ tiers: null, cls: '' }),
    thinkingTierDisabledReason: () => '',
  }
})
vi.mock('@/features/image-annotator/ImageAnnotator', () => ({
  ImageAnnotator: (props: typeof mocks.annotatorProps) => {
    mocks.annotatorProps = props
    return <div data-testid="image-annotator" />
  },
}))

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: Root[] = []
const pngHeader = new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10])
const agent = {
  id: 'AGT1',
  name: 'Agent',
  provider: 'anthropic',
  model: 'claude-sonnet-4',
} as Agent

function image(name: string, type = 'image/png') {
  const file = new File([pngHeader], name, { type })
  mockFileStream(file, pngHeader)
  return file
}

function mockFileStream(file: File, bytes: Uint8Array) {
  Object.defineProperty(file, 'stream', {
    value: () =>
      new ReadableStream<Uint8Array>({
        start(controller) {
          controller.enqueue(bytes)
          controller.close()
        },
      }),
  })
}
const attachment = (file: File) => ({
  id: `uploaded-${file.name}`,
  name: file.name,
  mime: file.type,
  kind: 'image' as const,
  size: file.size,
  relPath: `uploads/${file.name}`,
})

function renderComposer(sessionId = 'SES1') {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  const render = (nextSessionId = sessionId) =>
    act(() =>
      root.render(
        <Composer
          disabled={false}
          sessionId={nextSessionId}
          onSend={vi.fn()}
          agents={[agent]}
          agentId={agent.id}
          onAgentChange={vi.fn()}
          commands={[]}
        />,
      ),
    )
  render()
  return { container, root, render }
}

function paste(container: HTMLElement, files: File[]) {
  const event = new Event('paste', { bubbles: true, cancelable: true })
  Object.defineProperty(event, 'clipboardData', { value: { files, getData: () => '' } })
  act(() => container.querySelector('textarea')!.dispatchEvent(event))
  return event
}

function pickFiles(container: HTMLElement, files: File[]) {
  const input = container.querySelector<HTMLInputElement>('input[type="file"]')!
  Object.defineProperty(input, 'files', { configurable: true, value: files })
  act(() => input.dispatchEvent(new Event('change', { bubbles: true })))
}

async function flush() {
  await act(async () => {})
}

beforeEach(() => {
  mocks.annotatorProps = null
  mocks.uploadFile.mockImplementation(async (_sessionId: string, file: File) => attachment(file))
  mocks.deleteFile.mockResolvedValue(undefined)
  vi.stubGlobal(
    'createImageBitmap',
    vi.fn().mockResolvedValue({ width: 10, height: 10, close: vi.fn() }),
  )
  vi.stubGlobal('URL', {
    createObjectURL: vi.fn((file: File) => `blob:${file.name}`),
    revokeObjectURL: vi.fn(),
  })
})

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
  localStorage.clear()
  vi.unstubAllGlobals()
  vi.clearAllMocks()
})

describe('Composer clipboard integration', () => {
  it('opens selected images automatically in FIFO order and disposes each source', async () => {
    const closes = [vi.fn(), vi.fn()]
    const decoders: Array<(bitmap: ImageBitmap) => void> = []
    vi.mocked(createImageBitmap).mockImplementation(
      () => new Promise((resolve) => decoders.push(resolve)) as Promise<ImageBitmap>,
    )
    const { container } = renderComposer()
    pickFiles(container, [image('first.png'), image('second.png')])
    await flush()

    await act(async () => decoders[1]({ width: 10, height: 10, close: closes[1] } as ImageBitmap))
    expect(container.querySelector('[data-testid="image-annotator"]')).toBeNull()
    await act(async () => decoders[0]({ width: 10, height: 10, close: closes[0] } as ImageBitmap))

    expect(container.querySelector('[data-testid="image-annotator"]')).not.toBeNull()
    act(() => mocks.annotatorProps!.onClose())
    await flush()
    expect(closes[0]).toHaveBeenCalledOnce()
    expect(container.querySelector('[data-testid="image-annotator"]')).not.toBeNull()

    await act(async () =>
      mocks.annotatorProps!.onSave({
        blob: new Blob(['marked'], { type: 'image/png' }),
        mime: 'image/png',
        width: 10,
        height: 10,
      }),
    )
    expect(closes[1]).toHaveBeenCalledOnce()
    expect(mocks.uploadFile.mock.calls.map((call) => (call[1] as File).name)).toEqual([
      'second-annotated.png',
    ])
  })

  it('keeps non-image picker files on the normal upload path', async () => {
    const { container } = renderComposer()
    const text = new File(['hello'], 'notes.txt', { type: 'text/plain' })
    pickFiles(container, [text])
    await flush()

    expect(mocks.uploadFile).toHaveBeenCalledWith('SES1', text)
    expect(container.querySelector('[data-testid="image-annotator"]')).toBeNull()
  })

  it('surfaces picker image validation errors without uploading', async () => {
    const { container } = renderComposer()
    pickFiles(container, [image('bad.gif', 'image/gif')])
    await flush()
    await flush()

    expect(container.querySelector('[role="alert"]')?.textContent).toContain(
      'Bu görsel türü desteklenmiyor',
    )
    expect(mocks.uploadFile).not.toHaveBeenCalled()
  })

  it('edits an uploaded picker image again and safely replaces its pending attachment', async () => {
    const { container } = renderComposer()
    pickFiles(container, [image('picked.png')])
    await flush()

    await act(async () =>
      mocks.annotatorProps!.onSave({
        blob: new Blob([pngHeader], { type: 'image/png' }),
        mime: 'image/png',
        width: 10,
        height: 10,
      }),
    )
    mockFileStream(mocks.uploadFile.mock.calls[0][1] as File, pngHeader)

    const editButton = container.querySelector<HTMLButtonElement>('[aria-label="Görseli düzenle"]')
    expect(editButton).not.toBeNull()
    act(() => editButton!.click())
    await flush()
    await flush()
    expect(container.querySelector('[data-testid="image-annotator"]')).not.toBeNull()

    await act(async () =>
      mocks.annotatorProps!.onSave({
        blob: new Blob(['second edit'], { type: 'image/webp' }),
        mime: 'image/webp',
        width: 10,
        height: 10,
      }),
    )

    expect(mocks.uploadFile.mock.calls.map((call) => (call[1] as File).name)).toEqual([
      'picked-annotated.png',
      'picked-annotated-annotated.webp',
    ])
    expect(mocks.deleteFile).toHaveBeenCalledWith('uploads/picked-annotated.png')
    expect(container.querySelector('img')?.getAttribute('src')).toBe(
      'blob:picked-annotated-annotated.webp',
    )
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:picked-annotated.png')
    expect(container.querySelector('[data-testid="image-annotator"]')).toBeNull()
  })

  it('rolls back a picker image replacement when deleting its previous upload fails', async () => {
    const { container } = renderComposer()
    pickFiles(container, [image('picked.png')])
    await flush()
    await act(async () =>
      mocks.annotatorProps!.onSave({
        blob: new Blob([pngHeader], { type: 'image/png' }),
        mime: 'image/png',
        width: 10,
        height: 10,
      }),
    )
    mockFileStream(mocks.uploadFile.mock.calls[0][1] as File, pngHeader)

    act(() => container.querySelector<HTMLButtonElement>('[aria-label="Görseli düzenle"]')!.click())
    await flush()
    await flush()
    const deleteError = new Error('Seçilen eski upload silinemedi')
    mocks.deleteFile.mockRejectedValueOnce(deleteError).mockResolvedValueOnce(undefined)

    await expect(
      act(async () => {
        await mocks.annotatorProps!.onSave({
          blob: new Blob(['second edit'], { type: 'image/webp' }),
          mime: 'image/webp',
          width: 10,
          height: 10,
        })
      }),
    ).rejects.toThrow(deleteError)

    expect(mocks.deleteFile.mock.calls).toEqual([
      ['uploads/picked-annotated.png'],
      ['uploads/picked-annotated-annotated.webp'],
    ])
    expect(container.querySelector('[data-testid="image-annotator"]')).not.toBeNull()
    expect(container.querySelector('img')?.getAttribute('src')).toBe('blob:picked-annotated.png')
    expect(URL.revokeObjectURL).not.toHaveBeenCalledWith('blob:picked-annotated.png')
  })

  it('validates a clipboard image and directly uploads the original exactly once', async () => {
    const { container } = renderComposer()
    const original = image('screen.png')
    const event = paste(container, [original])
    expect(event.defaultPrevented).toBe(true)
    expect(mocks.uploadFile).not.toHaveBeenCalled()
    await flush()

    expect(container.querySelector('[aria-label="Yapıştırılan görsel seçimi"]')).toBeNull()
    expect(mocks.uploadFile).toHaveBeenCalledTimes(1)
    expect(mocks.uploadFile).toHaveBeenCalledWith('SES1', original)
  })

  it('opens annotator from the composer thumbnail and replaces the original upload', async () => {
    const { container } = renderComposer()
    const original = image('screen.png')
    paste(container, [original])
    await flush()
    expect(mocks.uploadFile).toHaveBeenCalledWith('SES1', original)

    act(() => container.querySelector<HTMLButtonElement>('[aria-label="Görseli düzenle"]')!.click())
    await flush()
    expect(container.querySelector('[data-testid="image-annotator"]')).not.toBeNull()

    const exported = new Blob(['annotated'], { type: 'image/webp' })
    await act(async () => {
      await mocks.annotatorProps!.onSave({
        blob: exported,
        mime: 'image/webp',
        width: 10,
        height: 10,
      })
    })

    expect(mocks.uploadFile).toHaveBeenCalledTimes(2)
    const uploaded = mocks.uploadFile.mock.calls[1][1] as File
    expect(uploaded).not.toBe(original)
    expect(uploaded.name).toBe('screen-annotated.webp')
    expect(uploaded.type).toBe('image/webp')
    expect(await uploaded.text()).toBe('annotated')
    expect(mocks.deleteFile).toHaveBeenCalledWith('uploads/screen.png')
    expect(container.querySelector('[data-testid="image-annotator"]')).toBeNull()
  })

  it('rolls back the replacement and keeps the editor open when deleting the original fails', async () => {
    const { container } = renderComposer()
    const original = image('screen.png')
    paste(container, [original])
    await flush()

    act(() => container.querySelector<HTMLButtonElement>('[aria-label="Görseli düzenle"]')!.click())
    await flush()
    const deleteError = new Error('Eski upload silinemedi')
    mocks.deleteFile.mockRejectedValueOnce(deleteError).mockResolvedValueOnce(undefined)

    await expect(
      act(async () => {
        await mocks.annotatorProps!.onSave({
          blob: new Blob(['annotated'], { type: 'image/webp' }),
          mime: 'image/webp',
          width: 10,
          height: 10,
        })
      }),
    ).rejects.toThrow(deleteError)

    expect(mocks.deleteFile.mock.calls).toEqual([
      ['uploads/screen.png'],
      ['uploads/screen-annotated.webp'],
    ])
    expect(container.querySelector('[data-testid="image-annotator"]')).not.toBeNull()
    expect(container.querySelector('img')?.getAttribute('src')).toBe('blob:screen.png')
    expect(URL.revokeObjectURL).not.toHaveBeenCalledWith('blob:screen.png')
  })

  it('keeps multiple pastes FIFO despite reverse decode completion', async () => {
    const decoders: Array<(bitmap: ImageBitmap) => void> = []
    vi.mocked(createImageBitmap).mockImplementation(
      () => new Promise((resolve) => decoders.push(resolve)) as Promise<ImageBitmap>,
    )
    const { container } = renderComposer()
    paste(container, [image('first.png')])
    paste(container, [image('second.png')])
    await flush()

    await act(async () => decoders[1]({ width: 10, height: 10, close: vi.fn() } as ImageBitmap))
    expect(mocks.uploadFile).not.toHaveBeenCalled()
    await act(async () => decoders[0]({ width: 10, height: 10, close: vi.fn() } as ImageBitmap))
    await flush()

    expect(mocks.uploadFile.mock.calls.map((call) => (call[1] as File).name)).toEqual([
      'first.png',
      'second.png',
    ])
  })

  it('ignores stale validation after session change and disposes bitmap', async () => {
    let resolveDecode!: (bitmap: ImageBitmap) => void
    vi.mocked(createImageBitmap).mockImplementation(
      () => new Promise((resolve) => (resolveDecode = resolve)) as Promise<ImageBitmap>,
    )
    const close = vi.fn()
    const { container, render } = renderComposer()
    paste(container, [image('stale.png')])
    await flush()
    render('SES2')
    await act(async () => resolveDecode({ width: 10, height: 10, close } as ImageBitmap))

    expect(close).toHaveBeenCalledTimes(1)
    expect(container.querySelector('[aria-label="Yapıştırılan görsel seçimi"]')).toBeNull()
    expect(mocks.uploadFile).not.toHaveBeenCalled()
  })

  it('does not mutate state after unmount while validation settles', async () => {
    let resolveDecode!: (bitmap: ImageBitmap) => void
    vi.mocked(createImageBitmap).mockImplementation(
      () => new Promise((resolve) => (resolveDecode = resolve)) as Promise<ImageBitmap>,
    )
    const close = vi.fn()
    const { container, root } = renderComposer()
    paste(container, [image('late.png')])
    await flush()
    act(() => root.unmount())
    roots.splice(roots.indexOf(root), 1)
    await act(async () => resolveDecode({ width: 10, height: 10, close } as ImageBitmap))

    expect(close).toHaveBeenCalledTimes(1)
    expect(mocks.uploadFile).not.toHaveBeenCalled()
  })

  it('ignores a stale upload result after session change and cleans its preview once', async () => {
    let resolveUpload!: (value: ReturnType<typeof attachment>) => void
    mocks.uploadFile.mockImplementation(() => new Promise((resolve) => (resolveUpload = resolve)))
    const { container, render } = renderComposer()
    const original = image('upload-race.png')
    paste(container, [original])
    await flush()
    render('SES2')

    await act(async () => resolveUpload(attachment(original)))

    expect(container.textContent).not.toContain('upload-race.png')
    expect(URL.revokeObjectURL).toHaveBeenCalledTimes(1)
  })

  it('creates and revokes each preview URL once on session cleanup', async () => {
    const { container, render } = renderComposer()
    paste(container, [image('preview.png')])
    await flush()

    expect(URL.createObjectURL).toHaveBeenCalledTimes(1)
    render('SES2')
    expect(URL.revokeObjectURL).toHaveBeenCalledTimes(1)
    render('SES3')
    expect(URL.revokeObjectURL).toHaveBeenCalledTimes(1)
  })

  it('surfaces validation and upload errors', async () => {
    const { container } = renderComposer()
    paste(container, [image('unknown', '')])
    await flush()
    expect(container.querySelector('[role="alert"]')?.textContent).toContain(
      'Bu görsel türü desteklenmiyor',
    )
    expect(mocks.uploadFile).not.toHaveBeenCalled()

    mocks.uploadFile.mockRejectedValueOnce(new Error('upload failed'))
    paste(container, [image('failure.png')])
    await flush()
    expect(container.textContent).toContain('upload failed')
    expect(mocks.uploadFile).toHaveBeenCalledTimes(1)
  })
})
