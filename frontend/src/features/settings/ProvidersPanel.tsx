import { useCallback, useEffect, useState } from 'react'
import { Boxes } from 'lucide-react'
import { api } from '@/api'
import type { ProviderInstance } from '@/api/providers'
import { toast } from '@/shared/components'
import { ClaudeAuthDialog } from './ClaudeAuthDialog'
import { CodexAuthDialog } from './CodexAuthDialog'
import { ProviderInstanceForm } from './providers/ProviderInstanceForm'
import { ProviderInstanceList, type ProviderAuthView } from './providers/ProviderInstanceList'
import { useProviderInstances } from './providers/useProviderInstances'
import { DeciderProviders } from '@/features/decider'
import { useTranslation } from 'react-i18next'

interface Props {
  // onOpenDecider switches to the Decision authorities screen.
  onOpenDecider?: () => void
}

function ProviderInstances() {
  const { t } = useTranslation('settings')
  const { kinds, instances, loading, error, upsert, remove } = useProviderInstances()
  const [editing, setEditing] = useState<ProviderInstance | null>(null)
  const [adding, setAdding] = useState(false)
  const [authById, setAuthById] = useState<Record<string, ProviderAuthView | undefined>>({})
  const [authInstance, setAuthInstance] = useState<ProviderInstance | null>(null)

  const refreshAuth = useCallback(
    async (instance: ProviderInstance) => {
      setAuthById((current) => ({ ...current, [instance.id]: 'pending' }))
      try {
        const status = await api.getAuth(instance.id)
        if (status.kind !== instance.kindId) {
          throw new Error(
            t('providers.authKindMismatch', { expected: instance.kindId, actual: status.kind }),
          )
        }
        setAuthById((current) => ({ ...current, [instance.id]: status }))
      } catch (caught) {
        setAuthById((current) => ({
          ...current,
          [instance.id]: { error: (caught as Error).message },
        }))
      }
    },
    [t],
  )

  useEffect(() => {
    for (const instance of instances) {
      if (instance.kindId === 'claude-cli' || instance.kindId === 'codex-cli') {
        void refreshAuth(instance)
      }
    }
  }, [instances, refreshAuth])

  if (loading) return <p className="text-xs text-[var(--color-text-dim)]">{t('common.loading')}</p>
  if (error) return <p className="text-xs text-[var(--color-danger)]">{error}</p>

  const cancel = () => {
    setEditing(null)
    setAdding(false)
  }

  const handleDelete = async (instance: ProviderInstance) => {
    if (!confirm(t('providers.deleteConfirm', { label: instance.label || instance.id }))) return
    try {
      const result = await remove(instance.id)
      if (result.affectedAgents.length > 0) {
        toast.warning(t('providers.deletedWithAgents', { count: result.affectedAgents.length }))
      } else {
        toast.success(t('providers.deleted'))
      }
      if (editing?.id === instance.id) cancel()
    } catch (caught) {
      toast.error((caught as Error).message)
    }
  }

  const selectedAuth = authInstance ? authById[authInstance.id] : undefined
  const isLoggedIn =
    selectedAuth !== undefined &&
    selectedAuth !== 'pending' &&
    'loggedIn' in selectedAuth &&
    selectedAuth.loggedIn

  return (
    <div className="space-y-2">
      {authInstance?.kindId === 'claude-cli' && (
        <ClaudeAuthDialog
          providerId={authInstance.id}
          providerLabel={authInstance.label || authInstance.id}
          isLoggedIn={isLoggedIn}
          onClose={() => setAuthInstance(null)}
          onLoggedIn={() => void refreshAuth(authInstance)}
        />
      )}
      {authInstance?.kindId === 'codex-cli' && (
        <CodexAuthDialog
          providerId={authInstance.id}
          providerLabel={authInstance.label || authInstance.id}
          isLoggedIn={isLoggedIn}
          onClose={() => setAuthInstance(null)}
          onLoggedIn={() => void refreshAuth(authInstance)}
        />
      )}

      {editing || adding ? (
        <ProviderInstanceForm
          kinds={kinds}
          instances={instances}
          editing={editing}
          onCancel={cancel}
          onSave={async (input) => {
            await upsert(input)
            toast.success(editing ? t('providers.updated') : t('providers.added'))
            cancel()
          }}
        />
      ) : (
        <button
          data-testid="provider-instance-add"
          onClick={() => {
            setEditing(null)
            setAdding(true)
          }}
          className="rounded border border-dashed border-[var(--color-border)] px-3 py-1.5 text-xs hover:border-[var(--color-accent)]"
        >
          {t('providers.add')}
        </button>
      )}

      <ProviderInstanceList
        instances={instances}
        kinds={kinds}
        authById={authById}
        onAuthOpen={setAuthInstance}
        onEdit={(instance) => {
          setEditing(instance)
          setAdding(false)
        }}
        onDelete={handleDelete}
      />
    </div>
  )
}

export function ProvidersPanel({ onOpenDecider }: Props) {
  const { t } = useTranslation('settings')
  return (
    <>
      <div className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        <Boxes size={13} className="text-[var(--color-accent)]" /> {t('providers.title')}
      </div>
      <p className="-mt-1 text-xs text-[var(--color-text-dim)]">{t('providers.description')}</p>
      <ProviderInstances />
      {/* Decision providers (internal/decider): answer typed questions only,
          so they live in their own list and never reach a model picker. */}
      <DeciderProviders onOpenAuthorities={onOpenDecider} />
    </>
  )
}
