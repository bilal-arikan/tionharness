import { useEffect, useState } from 'react'
import { api } from '@/api'
import { normalizeServerOS, type ServerOS } from '@/shared/lib/platform'

// Module-level cache: the server OS never changes for the life of the page, so
// GET /api/version is fetched once and shared by every consumer (same pattern as
// useCatalog in shared/lib/catalog.ts). A failed fetch clears the promise so the
// next mount retries.
let osCache: ServerOS | undefined
let osLoaded = false
let osPromise: Promise<ServerOS | undefined> | null = null

function loadServerOS(): Promise<ServerOS | undefined> {
  if (osLoaded) return Promise.resolve(osCache)
  if (!osPromise) {
    // Promise.resolve().then(...) so a missing/throwing getVersion (test mocks)
    // rejects instead of throwing synchronously inside a component effect.
    osPromise = Promise.resolve()
      .then(() => api.getVersion())
      .then((v) => {
        osCache = normalizeServerOS(v.os)
        osLoaded = true
        return osCache
      })
      .catch((e) => {
        osPromise = null
        throw e
      })
  }
  return osPromise
}

/** The cached server OS, or undefined while it loads / if it is unknown. */
export function getServerOS(): ServerOS | undefined {
  return osCache
}

/**
 * The OS family the backend runs on ('windows' | 'darwin' | 'linux'), or
 * undefined while loading or when the server did not report one. Callers must
 * treat undefined as "unknown" and keep a neutral/default behaviour.
 */
export function useServerOS(): ServerOS | undefined {
  const [os, setOS] = useState<ServerOS | undefined>(osCache)
  useEffect(() => {
    let alive = true
    loadServerOS()
      .then((v) => {
        if (alive) setOS(v)
      })
      .catch(() => {})
    return () => {
      alive = false
    }
  }, [])
  return os
}
