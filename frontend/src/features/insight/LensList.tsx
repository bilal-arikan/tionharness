import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Play, Pencil, X } from 'lucide-react'
import { api } from '@/api'
import type { InsightLens } from '@/types'
import { ModalOverlay, RestoreDefaultButton, SeedDefaultBadge, toast } from '@/shared/components'
import { ChannelBadge } from './insightBadges'

interface Props {
  lenses: InsightLens[]
  scanning: boolean
  onToggle: (id: string, enabled: boolean) => void
  onScanLens: (id: string) => void
  onSaved: () => void
  onError: (msg: string) => void
}

export function LensList({ lenses, scanning, onToggle, onScanLens, onSaved, onError }: Props) {
  const { t } = useTranslation('insight')
  const [editId, setEditId] = useState<string | null>(null)

  return (
    <div className="space-y-2">
      {lenses.map((l) => (
        <div
          key={l.id}
          className="flex items-center gap-3 rounded-md border border-[var(--color-border)] p-2"
        >
          <button
            type="button"
            role="switch"
            aria-checked={l.enabled}
            aria-label={t(l.enabled ? 'lenses.disableAria' : 'lenses.enableAria', { name: l.name })}
            title={t(l.enabled ? 'lenses.disable' : 'lenses.enable')}
            onClick={() => onToggle(l.id, !l.enabled)}
            className={`flex h-4 w-8 shrink-0 items-center rounded-full p-0.5 transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--color-accent)] ${
              l.enabled ? 'bg-[var(--color-accent)]' : 'bg-[var(--color-surface-2)]'
            }`}
          >
            <span
              aria-hidden="true"
              className={`h-3 w-3 rounded-full bg-[var(--color-surface)] transition-transform ${
                l.enabled ? 'translate-x-4' : 'translate-x-0'
              }`}
            />
          </button>
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <span className="font-medium">{l.name}</span>
              <ChannelBadge channel={l.channel} />
              <code className="text-xs text-[var(--color-text-dim)]">{l.id}</code>
              <SeedDefaultBadge state={l.defaultState} />
            </div>
            <div className="truncate text-xs text-[var(--color-text-dim)]">{l.description}</div>
          </div>
          <button
            onClick={() => onScanLens(l.id)}
            disabled={scanning}
            className="flex items-center gap-1 rounded px-2 py-1 text-xs hover:bg-[var(--color-surface-2)] disabled:opacity-50"
            title={t('lenses.scanOnlyTitle')}
          >
            <Play className="h-3.5 w-3.5" /> {t('scan.scan')}
          </button>
          <button
            onClick={() => setEditId(l.id)}
            className="flex items-center gap-1 rounded px-2 py-1 text-xs hover:bg-[var(--color-surface-2)]"
          >
            <Pencil className="h-3.5 w-3.5" /> {t('actions.edit')}
          </button>
          {/* Shipped lenses only: the automatic re-seed refreshes a lens ONLY
              when it can prove nobody edited it, so an edited (or pre-ledger)
              lens needs this deliberate opt-in to pick up shipped improvements. */}
          {l.defaultState && (
            <RestoreDefaultButton
              label={l.id}
              onRestore={() => api.restoreLens(l.id)}
              onDone={onSaved}
              onError={onError}
            />
          )}
        </div>
      ))}
      {lenses.length === 0 && (
        <div className="text-sm text-[var(--color-text-dim)]">{t('lenses.empty')}</div>
      )}

      {editId && (
        <LensEditor
          id={editId}
          onClose={() => setEditId(null)}
          onSaved={() => {
            setEditId(null)
            onSaved()
          }}
          onError={onError}
        />
      )}
    </div>
  )
}

function LensEditor({
  id,
  onClose,
  onSaved,
  onError,
}: {
  id: string
  onClose: () => void
  onSaved: () => void
  onError: (msg: string) => void
}) {
  const { t } = useTranslation('insight')
  const [raw, setRaw] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  // Lazy-load the raw file on first render.
  if (raw === null) {
    api
      .getLensRaw(id)
      .then((r) => setRaw(r.raw))
      .catch((e) => {
        onError((e as Error).message)
        onClose()
      })
  }

  const save = async () => {
    if (raw === null) return
    setSaving(true)
    try {
      await api.updateLens(id, raw)
      onSaved()
      toast.success(t('lenses.saved'))
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <ModalOverlay onClose={onClose}>
      <div className="flex max-h-[85vh] w-[min(800px,95vw)] flex-col rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
        <div className="mb-2 flex items-center justify-between">
          <h3 className="font-semibold">{t('lenses.editTitle', { id })}</h3>
          <button
            onClick={onClose}
            className="rounded p-1 hover:bg-[var(--color-surface-2)]"
            aria-label={t('actions.close')}
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <textarea
          value={raw ?? ''}
          onChange={(e) => setRaw(e.target.value)}
          spellCheck={false}
          className="min-h-[50vh] flex-1 rounded-md border border-[var(--color-border)] bg-[var(--color-surface-2)] p-2 font-mono text-xs"
        />
        <div className="mt-2 flex justify-end gap-2">
          <button
            onClick={onClose}
            className="rounded-md px-3 py-1 text-sm hover:bg-[var(--color-surface-2)]"
          >
            {t('actions.cancel')}
          </button>
          <button
            onClick={save}
            disabled={saving || raw === null}
            className="rounded-md bg-[var(--color-accent)] px-3 py-1 text-sm text-[var(--color-on-accent)] disabled:opacity-50"
          >
            {saving ? t('actions.saving') : t('actions.save')}
          </button>
        </div>
      </div>
    </ModalOverlay>
  )
}
