// The "Decision providers" section of Settings → Providers: decision models
// (internal/decider) listed, added and edited exactly like provider instances,
// but in their own list — they answer typed questions only, so they never show
// up in an agent's model picker. Which authority uses which one is set on the
// Decision authorities screen. Self managed: it loads /api/decider itself and
// every change saves at once.
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Scale } from 'lucide-react'
import { api } from '@/api'
import type {
  DeciderModelInput,
  DeciderModelInstance,
  DeciderTestResult,
  DeciderView,
} from '@/types/decider'
import { InfoPopover, toast } from '@/shared/components'
import { DeciderProviderCard } from './DeciderProviderCard'
import { DeciderProviderForm } from './DeciderProviderForm'
import { DeciderSetupGuide } from './DeciderSetupGuide'
import { setupSteps } from './deciderModel'
import { draftFromBackend, draftFromModel, draftFromPreset } from './modelDraft'
import { addButtonCls, smallButtonCls } from './providerStyles'

interface Props {
  // onOpenAuthorities switches to the Decision authorities screen.
  onOpenAuthorities?: () => void
}

type Editing = { input: DeciderModelInput; model?: DeciderModelInstance }

// A new decision provider starts on Jev through OpenRouter when that backend
// exists; the template select in the form offers the rest.
const PREFERRED_BACKEND = 'openrouter'
// QUICK_PRESET is what the setup guide's "add" button starts from: Jev through
// the OpenRouter account added above, no second key.
const QUICK_PRESET = 'jev-openrouter'

export function DeciderProviders({ onOpenAuthorities }: Props) {
  const { t } = useTranslation('decider')
  const [view, setView] = useState<DeciderView | null>(null)
  const [error, setError] = useState('')
  const [editing, setEditing] = useState<Editing | null>(null)

  useEffect(() => {
    api
      .getDecider()
      .then(setView)
      .catch((e) => setError((e as Error).message))
  }, [])

  const authorityLabel = (id: string) =>
    t(`authority.${id}.label`, {
      defaultValue: view?.authorities.find((a) => a.id === id)?.label ?? id,
    })

  // open starts a form. The view is reloaded first: a provider account added
  // above since this section loaded must be offered for borrowing.
  const open = async (next: Editing) => {
    try {
      setView(await api.getDecider())
    } catch {
      // The loaded view still works; only newer accounts would be missing.
    }
    setEditing(next)
  }

  if (error) return <p className="text-xs text-[var(--color-danger)]">{error}</p>
  if (!view) return <p className="text-xs text-[var(--color-text-dim)]">{t('providers.loading')}</p>

  const save = async (input: DeciderModelInput) => {
    const saved = editing?.model
      ? await api.updateDeciderModel(editing.model.id, input)
      : await api.createDeciderModel(input)
    setView(saved.view)
    toast.success(editing?.model ? t('providers.saved') : t('providers.added'))
    setEditing(null)
  }

  const remove = async (model: DeciderModelInstance) => {
    if (!confirm(t('providers.confirmDelete', { label: model.label || model.id }))) return
    try {
      const res = await api.deleteDeciderModel(model.id)
      setView(res.view)
      const users = res.usedBy.map((u) =>
        u === 'default' ? t('providers.usedByDefault') : authorityLabel(u),
      )
      if (users.length > 0) toast.info(t('providers.deletedUsed', { list: users.join(', ') }))
      else toast.success(t('providers.deleted'))
      if (editing?.model?.id === model.id) setEditing(null)
    } catch (e) {
      toast.error((e as Error).message)
    }
  }

  const test = async (model: DeciderModelInstance): Promise<DeciderTestResult> => {
    let res: DeciderTestResult
    try {
      res = await api.testDeciderModel(model.id)
    } catch (e) {
      return { ok: false, error: (e as Error).message }
    }
    // A test updates the provider's health (a failure may pause it); a failed
    // refresh only leaves the old badge up.
    api
      .getDecider()
      .then(setView)
      .catch(() => {})
    return res
  }

  const first = view.backends.find((b) => b.id === PREFERRED_BACKEND) ?? view.backends[0]
  const quick = view.backends.flatMap((b) =>
    (b.presets ?? []).filter((p) => p.id === QUICK_PRESET).map((p) => ({ backend: b, preset: p })),
  )[0]
  const blank = () =>
    first && {
      input: {
        ...draftFromBackend(first),
        label: t(`backend.${first.id}.label`, { defaultValue: first.label }),
      },
    }

  return (
    <>
      <div className="flex items-center gap-1.5 pt-4 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        <Scale size={13} className="text-[var(--color-accent)]" /> {t('providers.title')}
        <InfoPopover text={t('providers.hint')} />
      </div>
      {onOpenAuthorities && (
        <p className="-mt-1 text-xs">
          <button
            type="button"
            onClick={onOpenAuthorities}
            className="text-[var(--color-accent)] underline-offset-2 hover:underline"
            data-testid="decider-open-authorities"
          >
            {t('providers.openAuthorities')}
          </button>
        </p>
      )}
      <div className="space-y-2" data-testid="decider-providers">
        {!editing && (
          <DeciderSetupGuide
            steps={setupSteps(view)}
            actions={{
              provider: quick && (
                <button
                  type="button"
                  className={smallButtonCls}
                  onClick={() =>
                    void open({
                      input: {
                        ...draftFromPreset(quick.backend, quick.preset),
                        label: t(`preset.${quick.preset.id}.label`, {
                          defaultValue: quick.preset.label,
                        }),
                      },
                    })
                  }
                  data-testid="decider-setup-quick-add"
                >
                  {t('setup.provider.action', {
                    preset: t(`preset.${quick.preset.id}.label`, {
                      defaultValue: quick.preset.label,
                    }),
                  })}
                </button>
              ),
              enable: onOpenAuthorities && (
                <button
                  type="button"
                  onClick={onOpenAuthorities}
                  className="text-[var(--color-accent)] underline-offset-2 hover:underline"
                >
                  {t('setup.enable.goAuthorities')}
                </button>
              ),
            }}
          />
        )}
        {editing ? (
          <DeciderProviderForm
            key={editing.model?.id ?? 'new'}
            backends={view.backends}
            candidates={view.providerCandidates}
            initial={editing.input}
            editing={editing.model}
            onCancel={() => setEditing(null)}
            onSave={save}
          />
        ) : (
          first && (
            <button
              type="button"
              onClick={() => {
                const next = blank()
                if (next) void open(next)
              }}
              className={addButtonCls}
              data-testid="decider-provider-add"
            >
              {t('providers.add')}
            </button>
          )
        )}

        {view.models.length === 0 ? (
          <p className="text-xs text-[var(--color-text-dim)]">{t('providers.empty')}</p>
        ) : (
          <div className="grid gap-2">
            {view.models.map((m) => (
              <DeciderProviderCard
                key={m.id}
                model={m}
                backend={view.backends.find((b) => b.id === m.backend)}
                authorityLabel={authorityLabel}
                onEdit={() => void open({ input: draftFromModel(m), model: m })}
                onDelete={() => void remove(m)}
                onTest={() => test(m)}
              />
            ))}
          </div>
        )}
      </div>
    </>
  )
}
