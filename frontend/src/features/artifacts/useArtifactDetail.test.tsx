// @vitest-environment jsdom
//
// Covers the two correctness guards useArtifactDetail owns and that no rendered
// assertion elsewhere exercises:
//   1. the unsaved-draft guard (confirmDiscard) in selectArtifact / createNew,
//   2. the `cancelled` flag in both load effects (getArtifact + artifactPath),
//      on the resolve AND the reject path.

import { act, useState } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Artifact } from '@/types'
import { i18next } from '@/i18n'

const apiMock = vi.hoisted(() => ({
  getArtifact: vi.fn(),
  artifactPath: vi.fn(),
  createArtifact: vi.fn(),
  updateArtifact: vi.fn(),
  setArtifactGroup: vi.fn(),
  deleteArtifact: vi.fn(),
  setArtifactArchived: vi.fn(),
}))

vi.mock('@/api', () => ({ api: apiMock }))

import { useArtifactDetail, type ArtifactDetail } from './useArtifactDetail'

function artifact(over: Partial<Artifact> = {}): Artifact {
  return {
    id: 'ART1',
    sessionId: 'SES1',
    agentId: '',
    title: 'Başlık',
    kind: 'markdown',
    language: '',
    content: 'gövde',
    origin: 'manual',
    createdAt: 1,
    updatedAt: 1,
    ...over,
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  // The rejection is asserted through the hook's own catch; keep an inert
  // handler attached so an unhandled-rejection warning cannot fail the run.
  promise.catch(() => {})
  return { promise, resolve, reject }
}

// Module-scope handles onto the last render, the renderHook equivalent this repo
// has no library for (no @testing-library in devDependencies).
let detail: ArtifactDetail
let activeId: string | null
let setActiveId: (next: string | null) => void
let list: Artifact[]
const onError = vi.fn()

function Harness({ initialId }: { initialId: string | null }) {
  const [id, setId] = useState<string | null>(initialId)
  const [items, setItems] = useState<Artifact[]>([])
  activeId = id
  setActiveId = setId
  list = items
  detail = useArtifactDetail({ activeId: id, setActiveId: setId, setList: setItems, onError })
  return null
}

describe('useArtifactDetail', () => {
  let container: HTMLDivElement
  let root: Root | null

  beforeEach(() => {
    void i18next.changeLanguage('tr')
    vi.clearAllMocks()
    container = document.createElement('div')
    document.body.appendChild(container)
    root = null
    ;(
      globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true
    apiMock.getArtifact.mockResolvedValue(artifact())
    apiMock.artifactPath.mockResolvedValue({ path: '/store/ART1.md', dir: '/store' })
  })

  afterEach(async () => {
    if (root) await act(async () => root?.unmount())
    container.remove()
    vi.restoreAllMocks()
  })

  async function mount(initialId: string | null = 'ART1') {
    root = createRoot(container)
    await act(async () => {
      root!.render(<Harness initialId={initialId} />)
    })
  }

  // ---------------------------------------------------------------- dirty guard

  describe('unsaved-draft guard', () => {
    // Open the editor and change a field so `dirty` is true.
    async function makeDirty() {
      await act(async () => detail.startEdit())
      await act(async () => {
        detail.setDraft((d) => ({ ...d!, title: 'değişti' }))
      })
      expect(detail.dirty).toBe(true)
    }

    it('does not ask when the draft is clean and lets the selection through', async () => {
      await mount()
      await act(async () => detail.startEdit())
      expect(detail.dirty).toBe(false)

      const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
      await act(async () => detail.selectArtifact('ART2'))

      expect(confirmSpy).not.toHaveBeenCalled()
      expect(activeId).toBe('ART2')
    })

    it('keeps the current selection when the discard prompt is declined', async () => {
      await mount()
      await makeDirty()

      const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
      await act(async () => detail.selectArtifact('ART2'))

      expect(confirmSpy).toHaveBeenCalledOnce()
      expect(activeId).toBe('ART1')
    })

    it('changes the selection when the discard prompt is accepted', async () => {
      await mount()
      await makeDirty()

      vi.spyOn(window, 'confirm').mockReturnValue(true)
      await act(async () => detail.selectArtifact('ART2'))

      expect(activeId).toBe('ART2')
    })

    it('never calls createArtifact when the discard prompt is declined', async () => {
      await mount()
      await makeDirty()

      const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
      await act(async () => {
        await detail.createNew()
      })

      expect(confirmSpy).toHaveBeenCalledOnce()
      expect(apiMock.createArtifact).not.toHaveBeenCalled()
      expect(list).toHaveLength(0)
    })

    it('creates the artifact when the discard prompt is accepted', async () => {
      await mount()
      await makeDirty()
      apiMock.createArtifact.mockResolvedValue(artifact({ id: 'ART9', title: 'Yeni artifact' }))

      vi.spyOn(window, 'confirm').mockReturnValue(true)
      await act(async () => {
        await detail.createNew()
      })

      expect(apiMock.createArtifact).toHaveBeenCalledOnce()
      expect(activeId).toBe('ART9')
    })

    it('does not prompt on createNew while the draft is clean', async () => {
      await mount()
      apiMock.createArtifact.mockResolvedValue(artifact({ id: 'ART9' }))

      const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
      await act(async () => {
        await detail.createNew()
      })

      expect(confirmSpy).not.toHaveBeenCalled()
      expect(apiMock.createArtifact).toHaveBeenCalledOnce()
    })
  })

  // ------------------------------------------------------- late-response guards

  describe('superseded load responses', () => {
    it('drops a getArtifact response that resolves after the selection moved on', async () => {
      const first = deferred<Artifact>()
      const second = deferred<Artifact>()
      apiMock.getArtifact.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)

      await mount('ART1')
      await act(async () => setActiveId('ART2'))

      // The ART1 response lands only now — after its effect was cleaned up.
      await act(async () => {
        first.resolve(artifact({ id: 'ART1', title: 'bayat' }))
      })
      expect(detail.active).toBeNull()

      await act(async () => {
        second.resolve(artifact({ id: 'ART2', title: 'güncel' }))
      })
      expect(detail.active?.id).toBe('ART2')
    })

    it('drops a getArtifact rejection that lands after the selection moved on', async () => {
      const first = deferred<Artifact>()
      const second = deferred<Artifact>()
      apiMock.getArtifact.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)

      await mount('ART1')
      await act(async () => setActiveId('ART2'))

      await act(async () => {
        first.reject(new Error('bayat hata'))
      })
      expect(onError).not.toHaveBeenCalled()

      await act(async () => {
        second.resolve(artifact({ id: 'ART2' }))
      })
      expect(detail.active?.id).toBe('ART2')
    })

    it('drops an artifactPath response that resolves after the selection moved on', async () => {
      const first = deferred<{ path: string; dir: string }>()
      const second = deferred<{ path: string; dir: string }>()
      apiMock.artifactPath.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)

      await mount('ART1')
      await act(async () => setActiveId('ART2'))

      await act(async () => {
        first.resolve({ path: '/store/bayat.md', dir: '/store' })
      })
      expect(detail.activePath).toBe('')

      await act(async () => {
        second.resolve({ path: '/store/ART2.md', dir: '/store' })
      })
      expect(detail.activePath).toBe('/store/ART2.md')
    })

    it('drops an artifactPath rejection that lands after the selection moved on', async () => {
      const first = deferred<{ path: string; dir: string }>()
      const second = deferred<{ path: string; dir: string }>()
      apiMock.artifactPath.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)

      await mount('ART1')
      await act(async () => setActiveId('ART2'))
      await act(async () => {
        second.resolve({ path: '/store/ART2.md', dir: '/store' })
      })
      expect(detail.activePath).toBe('/store/ART2.md')

      // The superseded ART1 request fails only now. Clearing the path here would
      // blank the toolbar of the artifact currently on screen.
      await act(async () => {
        first.reject(new Error('bayat hata'))
      })
      expect(detail.activePath).toBe('/store/ART2.md')
    })
  })
})
