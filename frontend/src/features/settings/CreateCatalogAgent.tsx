import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { agentCatalogApi } from '@/api/agentCatalog'
import { ProviderInstanceModelSelect } from '@/shared/components/agents/ProviderInstanceModelSelect'
import { useCatalog, thinkingOptionsForModel } from '@/shared/lib/catalog'
import { THINKING_OPTIONS } from '@/features/agents/agentOptions'
import { InfoPopover } from '@/shared/components/InfoPopover'

interface Props {
  open: boolean
  onClose: () => void
  onCreated: (id: string) => void
}

export function CreateCatalogAgent({ open, onClose, onCreated }: Props) {
  const { t } = useTranslation('settingsMain')
  const [name, setName] = useState('')
  const [provider, setProvider] = useState('claude-cli')
  const [instance, setInstance] = useState('claude-cli')
  const [model, setModel] = useState('')
  const [level, setLevel] = useState('high')
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const catalog = useCatalog()
  const options = thinkingOptionsForModel(THINKING_OPTIONS, catalog, provider, model, level)
  const create = async () => {
    setPending(true)
    setError('')
    try {
      const agent = await agentCatalogApi.createCatalogAgent({
        name: name.trim(),
        provider: instance,
        model,
        thinkingLevel: level,
      })
      onClose()
      setName('')
      onCreated(agent.id)
    } catch (error) {
      setError((error as Error).message)
    } finally {
      setPending(false)
    }
  }
  if (!open) return null
  return (
    <div className="border-b border-[var(--color-border)] px-5 py-3">
      <form
        onSubmit={(event) => {
          event.preventDefault()
          void create()
        }}
        className="space-y-3 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
      >
        <label className="block text-xs">
          {t('createAgent.name')}
          <input
            autoFocus
            required
            maxLength={200}
            value={name}
            onChange={(event) => setName(event.target.value)}
            className="mt-1 block w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2"
          />
        </label>
        <ProviderInstanceModelSelect
          providerInstanceId={instance}
          model={model}
          onChange={(kind, id, nextModel) => {
            setProvider(kind)
            setInstance(id)
            setModel(nextModel)
          }}
        />
        <label className="block text-xs">
          {t('createAgent.reasoning')}
          <select
            value={level}
            onChange={(event) => setLevel(event.target.value)}
            className="ml-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2"
          >
            {options.map((option) => (
              <option key={option.value} value={option.value} disabled={option.disabled}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
        {error && (
          <p role="alert" className="text-xs text-[var(--color-danger)]">
            {error}
          </p>
        )}
        <div className="flex items-center gap-2">
          <button
            type="submit"
            disabled={pending || !name.trim()}
            className="rounded-lg bg-[var(--color-accent)] px-4 py-2 text-xs font-medium text-white disabled:opacity-50"
          >
            {pending ? t('createAgent.creating') : t('createAgent.submit')}
          </button>
          <InfoPopover text={t('createAgent.description')} />
        </div>
      </form>
    </div>
  )
}
