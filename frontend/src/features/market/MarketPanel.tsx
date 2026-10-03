import { useCallback, useEffect, useMemo, useState } from 'react'
import { useKeyedReset } from '@/shared/lib/useKeyedReset'
import { Download, RefreshCw, Store, Menu, Server } from 'lucide-react'
import type { Pack, PackKind, Secret } from '@/types'
import { api } from '@/api'
import type { PriceTable } from '@/api/providers'
import type { PreviewItem } from '@/api/ingest'
import { ListPane, LoadingState, toast } from '@/shared/components'
import { useCollapsibleList } from '@/shared/hooks/useCollapsibleList'
import { SkillImportDialog } from '@/features/skills/SkillImportDialog'
import { RegistryManager } from './RegistryManager'
import { KIND_NAV, emptyExisting, packTargetKey, type ExistingKeys } from './marketHelpers'
import { MarketGrid } from './MarketGrid'
import { PackDetailModal } from './PackDetailModal'
import { useTranslation } from 'react-i18next'
import { packKindPluralKey } from './marketHelpers'

interface Props {
  onError: (msg: string) => void
  // Jump to the Secrets screen to manage vault entries (provider keys live in
  // the vault, never typed as plaintext — mirrors the Settings providers panel).
  onManageSecrets?: () => void
  // Called after a successful install so the host can refresh the matching
  // collection (e.g. App-level agents state) without a full page reload.
  onInstalled?: (kind: PackKind) => void
}

export function MarketPanel({ onError, onManageSecrets, onInstalled }: Props) {
  const { t } = useTranslation('market')
  const [packs, setPacks] = useState<Pack[]>([])
  const { open: listOpen, toggle: toggleList } = useCollapsibleList('tionharness.marketListOpen')
  const [tab, setTab] = useState<PackKind>(KIND_NAV[0].key)
  const [selected, setSelected] = useState<Pack | null>(null)
  const [busy, setBusy] = useState(false)
  const [query, setQuery] = useState('') // catalog search (name/description/author)
  // Live directory-site (connector) search results — skill tab only.
  const [remoteResults, setRemoteResults] = useState<Pack[]>([])
  const [remoteWarnings, setRemoteWarnings] = useState<string[]>([])
  const [searching, setSearching] = useState(false)
  // Preview body for a selected source-ref pack (fetched on demand from GitHub).
  const [preview, setPreview] = useState<PreviewItem[] | null>(null)
  const [previewLoading, setPreviewLoading] = useState(false)
  const [installed, setInstalled] = useState<Set<string>>(new Set())
  const [existing, setExisting] = useState<ExistingKeys>(emptyExisting)
  // Provider key, resolved from the secret vault (never typed). pickedSecret is
  // the chosen secret's name (for display); apiKey holds its revealed value.
  const [secrets, setSecrets] = useState<Secret[]>([])
  const [pickedSecret, setPickedSecret] = useState('')
  const [apiKey, setApiKey] = useState('')
  // Skill import dialog (moved here from the Skills screen).
  const [importing, setImporting] = useState(false)
  // Remote registry manager modal ("Kaynaklar").
  const [managingRegistries, setManagingRegistries] = useState(false)
  // Ballpark list prices (provider id → model → price), loaded once for the
  // provider preview's per-model cost hints.
  const [prices, setPrices] = useState<PriceTable>({})

  // True until the first catalog fetch settles — the grid area shows a loading
  // state instead of an empty catalog.
  const [loading, setLoading] = useState(true)

  // The loaders land their results through callbacks, so the mount effect can
  // call them directly; loading starts true and clears with the catalog.
  const load = useCallback(
    () =>
      api
        .listMarket()
        .then(setPacks)
        .catch((e) => onError(e instanceof Error ? e.message : t('errors.loadFailed')))
        .finally(() => setLoading(false)),
    [onError, t],
  )

  const loadSecrets = useCallback(
    () =>
      api
        .listSecrets()
        .then(setSecrets)
        .catch(() => {
          // Secrets are optional; a load failure just means the picker is empty.
        }),
    [],
  )

  // loadExisting snapshots the workspace's current entities so the catalog can
  // flag packs that are already installed.
  const loadExisting = useCallback(
    () =>
      Promise.all([
        api.listSkills(),
        api.listAgents(),
        api.listProviders(),
        api.listWorkspaces(),
        api.listMCPServers(),
      ])
        .then(([skills, agents, providers, workspaces, mcp]) =>
          setExisting({
            skills: new Set(skills.map((s) => s.slug)),
            agents: new Set(agents.map((a) => a.name.trim().toLowerCase())),
            providers: new Set(providers.map((p) => p.id)),
            workspaces: new Set(workspaces.map((wsp) => wsp.name.trim().toLowerCase())),
            mcp: new Set(mcp.map((m) => m.name.trim().toLowerCase())),
          }),
        )
        .catch(() => {
          // Non-fatal: without this snapshot, packs simply aren't pre-marked.
        }),
    [],
  )

  useEffect(() => {
    void load()
    void loadSecrets()
    void loadExisting()
    void api
      .prices()
      .then(setPrices)
      .catch(() => {})
  }, [load, loadSecrets, loadExisting])

  // Live directory-site (connector) search — skill tab only, debounced. The sites
  // hold thousands of skills, so results come from a search query, not a bulk list.
  const trimmedQuery = query.trim()
  useKeyedReset(`${tab}|${trimmedQuery}`, () => {
    if (tab !== 'skill' || trimmedQuery.length < 2) {
      setRemoteResults([])
      setRemoteWarnings([])
      setSearching(false)
    } else {
      setSearching(true)
    }
  })
  useEffect(() => {
    const q = trimmedQuery
    if (tab !== 'skill' || q.length < 2) return
    const handle = setTimeout(async () => {
      try {
        const res = await api.searchConnectors(q)
        setRemoteResults(res.results ?? [])
        setRemoteWarnings(res.warnings ?? [])
      } catch {
        setRemoteResults([])
      } finally {
        setSearching(false)
      }
    }, 400)
    return () => clearTimeout(handle)
  }, [trimmedQuery, tab])

  // isInstalled reports whether the entity a pack would create already exists.
  const isInstalled = useCallback(
    (pack: Pack) => {
      const t = packTargetKey(pack)
      return t ? existing[t.set].has(t.key) : false
    },
    [existing],
  )

  // Resolve a chosen secret to its plaintext value (revealed on demand) and stage
  // it as the provider key for the next install.
  const pickSecret = useCallback(
    async (name: string) => {
      if (!name) {
        setPickedSecret('')
        setApiKey('')
        return
      }
      try {
        const { value } = await api.revealSecret(name)
        setPickedSecret(name)
        setApiKey(value)
      } catch (e) {
        onError(e instanceof Error ? e.message : t('errors.secretRevealFailed'))
      }
    },
    [onError, t],
  )

  const visible = useMemo(() => {
    const q = query.trim().toLowerCase()
    return packs.filter((p) => {
      if (p.kind !== tab) return false
      if (!q) return true
      return (
        p.name.toLowerCase().includes(q) ||
        (p.description ?? '').toLowerCase().includes(q) ||
        (p.author ?? '').toLowerCase().includes(q)
      )
    })
  }, [packs, tab, query])

  const openDetail = useCallback(
    async (pack: Pack) => {
      setApiKey('')
      setPickedSecret('')
      setPreview(null)
      // A source-ref pack (directory-site result) isn't in the catalog and has no
      // downloadable payload — use it as-is and fetch its preview from GitHub.
      if (pack.sourceRef?.url) {
        setSelected(pack)
        setPreviewLoading(true)
        try {
          const res = await api.ingestPreview({ source: 'github', url: pack.sourceRef.url })
          setPreview(res.items)
        } catch {
          setPreview([])
        } finally {
          setPreviewLoading(false)
        }
        return
      }
      try {
        setSelected(await api.getPack(pack.id))
      } catch (e) {
        onError(e instanceof Error ? e.message : t('errors.packOpenFailed'))
      }
    },
    [onError, t],
  )

  const reload = useCallback(async () => {
    setBusy(true)
    try {
      await api.refreshRegistries().catch(() => {}) // pull remote indexes (best-effort)
      await api.reloadMarket()
      await load()
      await loadExisting()
    } finally {
      setBusy(false)
    }
  }, [load, loadExisting])

  const install = useCallback(
    async (pack: Pack, overwrite = false) => {
      setBusy(true)
      try {
        // A source-ref pack (directory-site result) installs by ingesting its GitHub
        // source, not by downloading a payload.
        if (pack.sourceRef?.url) {
          const res = await api.ingestInstall({ source: 'github', url: pack.sourceRef.url })
          setInstalled((prev) => new Set(prev).add(pack.id))
          toast.success(res.message || t('status.installed'))
          await loadExisting()
          onInstalled?.('skill')
          return
        }
        const body: { overwrite?: boolean; apiKey?: string } = { overwrite }
        if (pack.kind === 'provider' && apiKey.trim()) body.apiKey = apiKey.trim()
        const res = await api.installPack(pack.id, body)
        setInstalled((prev) => new Set(prev).add(pack.id))
        toast.success(res.message || t('status.installed'))
        await loadExisting() // re-mark the catalog (this pack is now installed)
        await load() // refresh installedVersion decoration (ledger updated)
        onInstalled?.(pack.kind) // let the host refresh its matching collection
      } catch (e) {
        onError(e instanceof Error ? e.message : t('errors.installFailed'))
      } finally {
        setBusy(false)
      }
    },
    [onError, apiKey, loadExisting, load, onInstalled, t],
  )

  return (
    <div className="flex h-full">
      {/* Kind filter rail — standard ListPane column. */}
      <ListPane
        open={listOpen}
        onToggle={toggleList}
        widthKey="tionharness.marketListWidth"
        defaultWidth={200}
        minWidth={160}
        label={t('categories')}
        testId="market-list-toggle"
      >
        <div className="flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto p-2">
          <div className="flex items-center px-2 py-1">
            <span className="text-[10px] font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
              {t('categories')}
            </span>
          </div>
          {KIND_NAV.map((k) => {
            const Icon = k.icon
            const count = packs.filter((p) => p.kind === k.key).length
            const active = tab === k.key
            return (
              <button
                key={k.key}
                data-testid="market-kind-tab"
                data-kind={k.key}
                onClick={() => setTab(k.key)}
                className={`flex items-center gap-2.5 rounded-lg px-2.5 py-2 text-sm transition ${
                  active
                    ? 'bg-[var(--color-accent-soft)] font-medium text-[var(--color-accent)]'
                    : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]'
                }`}
              >
                <Icon size={16} strokeWidth={2} className="shrink-0" />
                <span className="min-w-0 flex-1 truncate text-left">
                  {t(packKindPluralKey(k.key))}
                </span>
                <span className="text-[10px] text-[var(--color-text-dim)]">{count}</span>
              </button>
            )
          })}
        </div>
        {/* Global market actions (moved here from the top header). */}
        <div className="flex flex-col gap-1 border-t border-[var(--color-border)] p-2">
          <button
            data-testid="market-import"
            onClick={() => setImporting(true)}
            title={t('actions.importTitle')}
            className="flex items-center gap-2 rounded-lg px-2.5 py-2 text-sm text-[var(--color-text-dim)] transition hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
          >
            <Download size={16} className="shrink-0" /> {t('actions.import')}
          </button>
          <button
            data-testid="market-registries"
            onClick={() => setManagingRegistries(true)}
            title={t('actions.sourcesTitle')}
            className="flex items-center gap-2 rounded-lg px-2.5 py-2 text-sm text-[var(--color-text-dim)] transition hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
          >
            <Server size={16} className="shrink-0" /> {t('actions.sources')}
          </button>
        </div>
      </ListPane>

      {/* Catalog */}
      <div className="flex flex-1 flex-col overflow-hidden">
        <header className="flex items-center justify-between border-b border-[var(--color-border)] px-5 py-3">
          <div className="flex min-w-0 items-center gap-2">
            <button
              onClick={toggleList}
              title={listOpen ? t('actions.hideList') : t('actions.showList')}
              aria-label={t('actions.toggleCategories')}
              data-testid="pane-list-toggle"
              className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-[var(--color-text-dim)] transition hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)] md:hidden"
            >
              <Menu size={18} />
            </button>
            <Store size={18} className="text-[var(--color-accent)]" />
            <h2 className="text-sm font-semibold">{t('title')}</h2>
            <span className="text-xs text-[var(--color-text-dim)]">
              {t('packCount', { count: visible.length })}
            </span>
          </div>
          <div className="flex items-center gap-1.5">
            <input
              data-testid="market-search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t('actions.searchPlaceholder')}
              className="w-40 rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] px-2.5 py-1 text-xs text-[var(--color-text)] outline-none focus:border-[var(--color-accent)]"
            />
            <button
              onClick={reload}
              disabled={busy}
              title={t('actions.refreshTitle')}
              className="flex items-center gap-1.5 rounded px-2 py-1 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] disabled:opacity-50"
            >
              <RefreshCw size={13} className={busy ? 'animate-spin' : ''} /> {t('actions.refresh')}
            </button>
          </div>
        </header>

        {loading && <LoadingState label={t('loading')} />}
        {!loading && (
          <MarketGrid
            visible={visible}
            tab={tab}
            selected={selected}
            installed={installed}
            isInstalled={isInstalled}
            openDetail={openDetail}
            searching={searching}
            remoteResults={remoteResults}
            remoteWarnings={remoteWarnings}
          />
        )}
      </div>

      {/* Detail popup (centered modal) */}
      {selected && (
        <PackDetailModal
          selected={selected}
          onClose={() => setSelected(null)}
          busy={busy}
          prices={prices}
          preview={preview}
          previewLoading={previewLoading}
          secrets={secrets}
          pickedSecret={pickedSecret}
          pickSecret={pickSecret}
          onManageSecrets={onManageSecrets}
          isInstalled={isInstalled}
          install={install}
        />
      )}

      {importing && (
        <SkillImportDialog
          onClose={() => setImporting(false)}
          onImported={(kinds) => {
            void loadExisting() // re-mark installed entities in the catalog
            kinds.forEach((k) => onInstalled?.(k)) // let the host refresh matching collections
          }}
        />
      )}

      {managingRegistries && (
        <RegistryManager
          onClose={() => setManagingRegistries(false)}
          onChanged={() => void reload()} // re-fetch remote indexes + catalog
        />
      )}
    </div>
  )
}
