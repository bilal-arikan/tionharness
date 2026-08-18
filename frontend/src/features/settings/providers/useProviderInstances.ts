// useProviderInstances loads the provider-kind catalog and the configured
// instance list (both app-global, independent of the main settings save
// flow) and exposes CRUD mutators over the dedicated /api/providers and
// /api/provider-kinds endpoints.
import { useCallback, useEffect, useState } from 'react'
import { api } from '@/api'
import type { ProviderKind, ProviderInstance, UpsertProviderInput } from '@/api/providers'

export function useProviderInstances() {
  const [kinds, setKinds] = useState<ProviderKind[]>([])
  const [instances, setInstances] = useState<ProviderInstance[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const reload = useCallback(async () => {
    try {
      const [k, i] = await Promise.all([api.listProviderKinds(), api.listProviders()])
      setKinds(k)
      setInstances(i)
      setError('')
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    reload()
  }, [reload])

  const upsert = useCallback(async (input: UpsertProviderInput) => {
    const saved = await api.upsertProvider(input)
    setInstances((prev) => {
      const idx = prev.findIndex((p) => p.id === saved.id)
      if (idx === -1) return [...prev, saved]
      const next = prev.slice()
      next[idx] = saved
      return next
    })
    return saved
  }, [])

  const remove = useCallback(async (id: string) => {
    const result = await api.deleteProvider(id)
    setInstances((prev) => prev.filter((p) => p.id !== id))
    return result
  }, [])

  return { kinds, instances, loading, error, reload, upsert, remove }
}
