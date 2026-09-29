import { Download, Check, KeyRound, ArrowUpCircle, X } from 'lucide-react'
import type { Pack, Secret } from '@/types'
import type { PriceTable } from '@/api/providers'
import type { PreviewItem } from '@/api/ingest'
import { Button, ModalOverlay } from '@/shared/components'
import { installLabelKey, updateAvailable } from './marketHelpers'
import { KindBadge, PackMeta, SourceRefPreview } from './previewParts'
import { PackPreview } from './PackPreview'
import { useTranslation } from 'react-i18next'

interface Props {
  selected: Pack
  onClose: () => void
  busy: boolean
  prices: PriceTable
  // Preview body for a selected source-ref pack (fetched on demand from GitHub).
  preview: PreviewItem[] | null
  previewLoading: boolean
  // Provider key picker state — secrets list plus the currently picked name.
  secrets: Secret[]
  pickedSecret: string
  pickSecret: (name: string) => Promise<void>
  onManageSecrets?: () => void
  isInstalled: (pack: Pack) => boolean
  install: (pack: Pack, overwrite?: boolean) => Promise<void>
}

// PackDetailModal is the centered detail popup for a selected pack: header,
// metadata, provider key picker, install/update button and payload preview.
export function PackDetailModal({
  selected,
  onClose,
  busy,
  prices,
  preview,
  previewLoading,
  secrets,
  pickedSecret,
  pickSecret,
  onManageSecrets,
  isInstalled,
  install,
}: Props) {
  const { t } = useTranslation('market')
  return (
    <ModalOverlay onClose={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-label={selected.name}
        data-testid="market-detail-modal"
        className="flex max-h-[88vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl"
      >
        <header className="flex items-start justify-between gap-2 border-b border-[var(--color-border)] px-4 py-3">
          <div className="flex items-center gap-2">
            <span className="text-xl">{selected.icon || '📦'}</span>
            <div>
              <div className="text-sm font-semibold">{selected.name}</div>
              <div className="flex items-center gap-1.5 text-[10px] text-[var(--color-text-dim)]">
                <KindBadge kind={selected.kind} />
                {selected.author && <span>{t('detail.author', { author: selected.author })}</span>}
                {selected.version && (
                  <span>{t('detail.versionWithSeparator', { version: selected.version })}</span>
                )}
              </div>
            </div>
          </div>
          <button
            onClick={onClose}
            title={t('actions.close')}
            aria-label={t('actions.close')}
            className="text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
          >
            <X size={16} />
          </button>
        </header>

        <div className="border-b border-[var(--color-border)] p-4">
          <p className="text-xs text-[var(--color-text-dim)]">{selected.description}</p>
          <PackMeta pack={selected} />
          {selected.kind === 'provider' && (
            <div className="mt-3">
              <label className="text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
                {t('detail.apiKeyFromSecrets')}
              </label>
              <div className="mt-1 flex items-center gap-2">
                <select
                  value={pickedSecret}
                  onChange={(e) => void pickSecret(e.target.value)}
                  disabled={secrets.length === 0}
                  className="flex-1 rounded border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-1.5 text-xs"
                >
                  <option value="">
                    {secrets.length ? t('detail.chooseSecret') : t('detail.noSecrets')}
                  </option>
                  {secrets.map((s) => (
                    <option key={s.name} value={s.name}>
                      {s.name}
                    </option>
                  ))}
                </select>
                {onManageSecrets && (
                  <button
                    onClick={onManageSecrets}
                    className="flex items-center gap-1 rounded border border-[var(--color-border)] px-2 py-1.5 text-xs hover:border-[var(--color-accent)]"
                  >
                    <KeyRound size={12} /> {t('detail.secrets')}
                  </button>
                )}
              </div>
              <p className="mt-1 text-[10px] text-[var(--color-text-dim)]">
                {pickedSecret
                  ? t('detail.secretWillBeUsed', { name: pickedSecret })
                  : t('detail.keyOptionalHint')}
              </p>
            </div>
          )}
          {(() => {
            const here = isInstalled(selected)
            const canUpdate = updateAvailable(selected)
            // Providers are id-keyed (Upsert) so re-installing just updates the
            // config/key — allowed and labelled "Güncelle". A pack with a newer
            // version than the one recorded in the ledger is always updatable
            // (overwrite). Other kinds with identity are blocked once present to
            // avoid duplicates.
            const blocked = here && selected.kind !== 'provider' && !canUpdate
            const label = canUpdate
              ? t('grid.updateVersion', {
                  from: selected.installedVersion,
                  to: selected.version,
                })
              : blocked
                ? t('status.alreadyInstalled')
                : here && selected.kind === 'provider'
                  ? t('actions.update')
                  : t(installLabelKey(selected.kind))
            return (
              <div
                data-testid="market-pack-install"
                data-pack-id={selected.id}
                className="contents"
              >
                <Button
                  onClick={() => void install(selected, canUpdate)}
                  disabled={busy || blocked}
                  className="mt-3 w-full"
                >
                  {canUpdate ? (
                    <ArrowUpCircle size={13} />
                  ) : blocked ? (
                    <Check size={13} />
                  ) : (
                    <Download size={13} />
                  )}{' '}
                  {label}
                </Button>
              </div>
            )
          })()}
        </div>

        {/* Payload preview — kind-specific, or a fetched GitHub preview for source-ref */}
        <div className="flex-1 overflow-y-auto p-4">
          {selected.sourceRef?.url ? (
            <SourceRefPreview
              url={selected.sourceRef.url}
              items={preview}
              loading={previewLoading}
            />
          ) : (
            <PackPreview pack={selected} prices={prices} />
          )}
        </div>
      </div>
    </ModalOverlay>
  )
}
