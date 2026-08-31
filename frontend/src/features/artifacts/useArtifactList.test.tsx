// @vitest-environment jsdom
//
// Covers the list request race guard in useArtifactList: every fetchPage stamps
// a sequence number and drops its own response once a newer request has been
// issued. Filters change fast (300 ms search debounce, origin facet, archive
// toggle), so a slow earlier page must never overwrite a newer one.

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ArtifactPage } from '@/api/artifacts'
import type { Artifact } from '@/types'
import { clearSessionState } from '@/shared/hooks/useSessionState'

const apiMock = vi.hoisted(() => ({
  listArtifacts: vi.fn(),
}))

vi.mock('@/api', () => ({ api: apiMock }))

import { useArtifactList, type ArtifactList } from './useArtifactList'

function artifact(id: string): Artifact {
  return {
    id,
    sessionId: 'SES1',
    agentId: '',
    title: id,
    kind: 'markdown',
    language: '',
    content: '',
    origin: 'manual',
    createdAt: 1,
    updatedAt: 1,
  }
}

function page(items: Artifact[]): ArtifactPage {
  return { items, total: items.length, offset: 0, limit: 50, hasMore: false }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((res) => {
    resolve = res
  })
  return { promise, resolve }
}

// Deferreds for the paged list requests, in call order. The archive-badge probe
// (`{ archived: true, limit: 1 }`) is answered inline so it never shifts the
// indices the race assertions depend on.
let pages: Array<ReturnType<typeof deferred<ArtifactPage>>>

let hook: ArtifactList
const onError = vi.fn()

function Harness() {
  hook = useArtifactList(null, 0, onError)
  return null
}

describe('useArtifactList request race guard', () => {
  let container: HTMLDivElement
  let root: Root | null

  beforeEach(() => {
    vi.clearAllMocks()
    clearSessionState()
    localStorage.clear()
    pages = []
    apiMock.listArtifacts.mockImplementation(
      (params: { archived?: boolean; limit?: number } = {}) => {
        if (params.archived === true && params.limit === 1) return Promise.resolve(page([]))
        const d = deferred<ArtifactPage>()
        pages.push(d)
        return d.promise
      },
    )
    container = document.createElement('div')
    document.body.appendChild(container)
    root = null
    ;(
      globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true
  })

  afterEach(async () => {
    if (root) await act(async () => root?.unmount())
    container.remove()
    vi.restoreAllMocks()
  })

  async function mount() {
    root = createRoot(container)
    await act(async () => {
      root!.render(<Harness />)
    })
  }

  it('drops a stale page that resolves after a newer request was issued', async () => {
    await mount()
    expect(pages).toHaveLength(1)

    // Second request supersedes the first while it is still in flight.
    await act(async () => hook.reload())
    expect(pages).toHaveLength(2)

    // The first (now stale) page lands: neither the list nor the selection may move.
    await act(async () => {
      pages[0].resolve(page([artifact('STALE1'), artifact('STALE2')]))
    })
    expect(hook.list).toEqual([])
    expect(hook.activeId).toBeNull()
    expect(hook.total).toBe(0)

    // The current request lands and is applied.
    await act(async () => {
      pages[1].resolve(page([artifact('FRESH1')]))
    })
    expect(hook.list.map((a) => a.id)).toEqual(['FRESH1'])
    expect(hook.activeId).toBe('FRESH1')
    expect(hook.total).toBe(1)
  })

  it('keeps the newest page when the stale one resolves last', async () => {
    await mount()
    await act(async () => hook.reload())

    await act(async () => {
      pages[1].resolve(page([artifact('FRESH1')]))
    })
    expect(hook.list.map((a) => a.id)).toEqual(['FRESH1'])

    await act(async () => {
      pages[0].resolve(page([artifact('STALE1')]))
    })
    expect(hook.list.map((a) => a.id)).toEqual(['FRESH1'])
    expect(hook.activeId).toBe('FRESH1')
  })
})
