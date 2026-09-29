import { useEffect, useState } from 'react'
import { Eye, EyeOff, Copy, Trash2, KeyRound } from 'lucide-react'
import { api } from '@/api'
import type { Secret } from '@/types'
import { copyToClipboard } from '@/shared/lib/clipboard'
import { Button, LoadingState, toast } from '@/shared/components'
import { useAsync } from '@/shared/hooks/useAsync'
import { Trans, useTranslation } from 'react-i18next'

interface Props {
  onError: (msg: string) => void
}

// SecretsPanel is the per-workspace secret vault screen. The user stores
// credentials (API keys, tokens, passwords) here; values are AES-GCM encrypted
// on the backend and never shown in the list. Agents in this workspace read them
// through the secret_list / secret_get tools. A value can be revealed/copied on
// demand via an explicit per-secret action.
export function SecretsPanel({ onError }: Props) {
  const { t } = useTranslation('settings')
  // Load the workspace secret list; refetched after add/edit/delete. Errors
  // surface via onError.
  const { data, loading, error, refresh: load } = useAsync(() => api.listSecrets(), [])
  const secrets = data ?? []
  useEffect(() => {
    if (error) onError(error)
  }, [error, onError])

  // Add / edit form.
  const [name, setName] = useState('')
  const [value, setValue] = useState('')
  const [description, setDescription] = useState('')
  const [editing, setEditing] = useState(false)
  const [saving, setSaving] = useState(false)

  // Revealed values, keyed by secret name (cleared on hide).
  const [revealed, setRevealed] = useState<Record<string, string>>({})

  const resetForm = () => {
    setName('')
    setValue('')
    setDescription('')
    setEditing(false)
  }

  const save = async () => {
    const n = name.trim()
    if (!n || !value) return
    setSaving(true)
    try {
      await api.setSecret(n, value, description.trim())
      const wasEditing = editing
      resetForm()
      load()
      toast.success(wasEditing ? t('secrets.updated') : t('secrets.added'))
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  // startEdit pre-fills the form name/description for an update; the value field
  // stays empty (write-only) and must be re-entered to change it.
  const startEdit = (s: Secret) => {
    setName(s.name)
    setValue('')
    setDescription(s.description)
    setEditing(true)
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  const toggleReveal = async (s: Secret) => {
    if (revealed[s.name] !== undefined) {
      setRevealed((r) => {
        const next = { ...r }
        delete next[s.name]
        return next
      })
      return
    }
    try {
      const res = await api.revealSecret(s.name)
      setRevealed((r) => ({ ...r, [s.name]: res.value }))
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const copyValue = async (s: Secret) => {
    try {
      const val = revealed[s.name] ?? (await api.revealSecret(s.name)).value
      if (await copyToClipboard(val)) toast.info(t('secrets.copied'))
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const remove = async (s: Secret) => {
    if (!confirm(t('secrets.deleteConfirm', { name: s.name }))) return
    try {
      await api.deleteSecret(s.name)
      load()
      toast.success(t('secrets.deleted'))
    } catch (e) {
      onError((e as Error).message)
    }
  }

  return (
    <div className="min-h-0 flex-1 overflow-y-auto p-6">
      <div className="mx-auto max-w-3xl space-y-8">
        <header>
          <div className="flex items-center gap-2">
            <KeyRound size={18} className="text-[var(--color-accent)]" />
            <h2 className="text-sm font-semibold">{t('secrets.title')}</h2>
          </div>
          <p className="mt-1 text-xs text-[var(--color-text-dim)]">
            <Trans
              i18nKey="secrets.intro"
              ns="settings"
              components={{
                strong: <strong />,
                secretList: <code className="rounded bg-[var(--color-surface-2)] px-1 py-0.5" />,
                secretGet: <code className="rounded bg-[var(--color-surface-2)] px-1 py-0.5" />,
              }}
            />
          </p>
        </header>

        {/* Add / edit form */}
        <section className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
          <h3 className="mb-3 text-xs font-semibold text-[var(--color-text-dim)]">
            {editing ? t('secrets.editTitle', { name }) : t('secrets.newTitle')}
          </h3>
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
            <input
              data-testid="secret-name-input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={editing}
              placeholder={t('secrets.namePlaceholder')}
              className="rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none disabled:opacity-60"
            />
            <input
              data-testid="secret-description-input"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder={t('secrets.descriptionPlaceholder')}
              className="rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none"
            />
            <input
              data-testid="secret-value-input"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              type="password"
              autoComplete="new-password"
              placeholder={
                editing ? t('secrets.newValuePlaceholder') : t('secrets.valuePlaceholder')
              }
              className="rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none sm:col-span-2"
            />
          </div>
          <p className="mt-2 text-xs text-[var(--color-text-dim)]">
            {t('secrets.nameRulePrefix')} <code>_</code>, <code>-</code>, <code>.</code>{' '}
            {t('secrets.nameRuleSuffix')}
          </p>
          <div className="mt-3 flex items-center gap-2">
            <Button
              data-testid="secret-save"
              onClick={save}
              disabled={saving || !name.trim() || !value}
              size="lg"
            >
              {editing ? t('common.update') : t('common.add')}
            </Button>
            {editing && (
              <Button onClick={resetForm} variant="secondary" size="lg">
                {t('common.cancel')}
              </Button>
            )}
          </div>
        </section>

        {/* Secret list */}
        <section>
          <h3 className="mb-2 text-sm font-semibold">
            {t('secrets.stored')}
            {secrets.length > 0 && ` (${secrets.length})`}
          </h3>
          <div className="divide-y divide-[var(--color-border)] overflow-hidden rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)]">
            {secrets.map((s) => (
              <div key={s.name} className="px-4 py-3">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <code className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-xs font-medium">
                      {s.name}
                    </code>
                    {s.description && (
                      <span className="ml-2 text-sm text-[var(--color-text-dim)]">
                        {s.description}
                      </span>
                    )}
                    <div className="mt-1.5 font-mono text-xs text-[var(--color-text-dim)]">
                      {revealed[s.name] !== undefined ? (
                        <span className="break-all text-[var(--color-text)]">
                          {revealed[s.name]}
                        </span>
                      ) : (
                        '••••••••••••'
                      )}
                    </div>
                  </div>
                  <div className="flex flex-shrink-0 items-center gap-1">
                    <button
                      data-testid="secret-reveal"
                      data-secret-name={s.name}
                      onClick={() => toggleReveal(s)}
                      title={revealed[s.name] !== undefined ? t('common.hide') : t('common.show')}
                      className="rounded p-1.5 text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
                    >
                      {revealed[s.name] !== undefined ? <EyeOff size={15} /> : <Eye size={15} />}
                    </button>
                    <button
                      data-testid="secret-copy"
                      data-secret-name={s.name}
                      onClick={() => copyValue(s)}
                      title={t('common.copy')}
                      className="rounded p-1.5 text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
                    >
                      <Copy size={15} />
                    </button>
                    <button
                      onClick={() => startEdit(s)}
                      className="rounded px-2 py-1 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
                    >
                      {t('common.edit')}
                    </button>
                    <button
                      data-testid="secret-delete"
                      data-secret-name={s.name}
                      onClick={() => remove(s)}
                      title={t('common.delete')}
                      className="rounded p-1.5 text-[var(--color-danger)] hover:bg-[var(--color-danger)]/10"
                    >
                      <Trash2 size={15} />
                    </button>
                  </div>
                </div>
              </div>
            ))}
            {!loading && secrets.length === 0 && (
              <p className="px-4 py-6 text-center text-sm text-[var(--color-text-dim)]">
                {t('secrets.empty')}
              </p>
            )}
            {loading && <LoadingState label={t('common.loading')} />}
          </div>
        </section>
      </div>
    </div>
  )
}
