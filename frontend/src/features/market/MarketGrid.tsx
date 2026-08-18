import { Check, ArrowUpCircle, Globe2 } from 'lucide-react'
import type { Pack, PackKind } from '@/types'
import { updateAvailable } from './marketHelpers'
import { KindBadge } from './previewParts'

interface Props {
  visible: Pack[]
  tab: PackKind
  selected: Pack | null
  installed: Set<string>
  isInstalled: (pack: Pack) => boolean
  openDetail: (pack: Pack) => Promise<void>
  // Live directory-site (connector) search results — skill tab only.
  searching: boolean
  remoteResults: Pack[]
  remoteWarnings: string[]
}

// MarketGrid is the browsable catalog grid: filtered local/remote catalog packs
// plus the live directory-site (connector) search results on the skill tab.
export function MarketGrid({
  visible,
  tab,
  selected,
  installed,
  isInstalled,
  openDetail,
  searching,
  remoteResults,
  remoteWarnings,
}: Props) {
  return (
    <div className="grid flex-1 grid-cols-[repeat(auto-fill,minmax(240px,1fr))] content-start gap-3 overflow-y-auto p-4">
      {visible.length === 0 && (
        <p className="col-span-full mt-8 text-center text-sm text-[var(--color-text-dim)]">
          Bu türde paket yok.
        </p>
      )}
      {visible.map((p) => {
        const done = installed.has(p.id) || isInstalled(p)
        return (
          <button
            key={p.id}
            data-testid="market-pack"
            data-pack-id={p.id}
            onClick={() => void openDetail(p)}
            className={`flex flex-col gap-2 rounded-lg border p-3 text-left transition hover:border-[var(--color-accent)] ${
              selected?.id === p.id
                ? 'border-[var(--color-accent)] bg-[var(--color-surface-2)]'
                : 'border-[var(--color-border)]'
            }`}
          >
            <div className="flex items-center gap-2">
              <span
                className="flex h-8 w-8 shrink-0 items-center justify-center rounded text-lg"
                style={{ background: p.color ? `${p.color}22` : 'var(--color-surface-2)' }}
              >
                {p.icon || '📦'}
              </span>
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm font-medium">{p.name}</div>
                <div className="flex items-center gap-1.5">
                  <KindBadge kind={p.kind} />
                  {p.version && (
                    <span className="text-[10px] text-[var(--color-text-dim)]">v{p.version}</span>
                  )}
                </div>
              </div>
            </div>
            <p className="line-clamp-3 text-xs text-[var(--color-text-dim)]">{p.description}</p>
            <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
              <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[9px] uppercase tracking-wide text-[var(--color-text-dim)]">
                {p.source === 'remote' ? p.registryName || 'Uzak' : 'Yerel'}
              </span>
              {updateAvailable(p) && (
                <span className="flex items-center gap-1 text-[10px] text-[var(--color-accent)]">
                  <ArrowUpCircle size={11} /> Güncelle (v{p.installedVersion}→v{p.version})
                </span>
              )}
              {done && !updateAvailable(p) && (
                <span className="flex items-center gap-1 text-[10px] text-[var(--color-success)]">
                  <Check size={11} /> Kuruldu
                </span>
              )}
            </div>
          </button>
        )
      })}

      {/* Live directory-site search results (skill tab) */}
      {tab === 'skill' && (searching || remoteResults.length > 0 || remoteWarnings.length > 0) && (
        <div className="col-span-full mt-2 border-t border-[var(--color-border)] pt-3">
          <div className="mb-2 flex items-center gap-2 text-xs font-medium text-[var(--color-text-dim)]">
            <Globe2 size={13} />
            İnternet sonuçları (SkillsMP · CrossAITools)
            {searching && <span className="text-[var(--color-text-dim)]">aranıyor…</span>}
            {!searching && <span>· {remoteResults.length}</span>}
          </div>
          {remoteWarnings.length > 0 && (
            <p className="mb-2 text-[10px] text-[var(--color-warning,#d97706)]">
              {remoteWarnings.join(' · ')}
            </p>
          )}
        </div>
      )}
      {tab === 'skill' &&
        remoteResults.map((p) => {
          const done = installed.has(p.id)
          return (
            <button
              key={p.id}
              data-testid="market-remote-pack"
              data-pack-id={p.id}
              onClick={() => void openDetail(p)}
              className={`flex flex-col gap-2 rounded-lg border p-3 text-left transition hover:border-[var(--color-accent)] ${
                selected?.id === p.id
                  ? 'border-[var(--color-accent)] bg-[var(--color-surface-2)]'
                  : 'border-[var(--color-border)]'
              }`}
            >
              <div className="flex items-center gap-2">
                <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded text-lg bg-[var(--color-surface-2)]">
                  {p.icon || '🌐'}
                </span>
                <div className="min-w-0 flex-1">
                  <div className="truncate text-sm font-medium">{p.name}</div>
                  <KindBadge kind={p.kind} />
                </div>
              </div>
              <p className="line-clamp-3 text-xs text-[var(--color-text-dim)]">{p.description}</p>
              <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[9px] uppercase tracking-wide text-[var(--color-text-dim)]">
                  {p.registryName || 'Uzak'}
                </span>
                {done && (
                  <span className="flex items-center gap-1 text-[10px] text-[var(--color-success)]">
                    <Check size={11} /> Kuruldu
                  </span>
                )}
              </div>
            </button>
          )
        })}
    </div>
  )
}
