// Codex plugin configuration for one workspace: the marketplaces it resolves
// plugin selectors against, which plugins are enabled, and a one-click import of
// the marketplaces already installed on this machine.
//
// The import exists because codex reserves its own marketplace names
// ("openai-bundled" and the remote catalogs) and refuses to register them from
// any other source — a workspace can only use them as a renamed copy it owns.
// That copy is never made automatically: those are OpenAI's proprietary files,
// so it happens only when the user asks for it here.
import { useState } from 'react'
import { Download, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { workspaceApi } from '@/api/workspaces'
import { Button, toast } from '@/shared/components'
import type { CodexDiscoveredMarketplace, CodexMarketplace } from '@/types'
import { inputCls } from './primitives'

interface Props {
  marketplaces: CodexMarketplace[]
  plugins: string[]
  onChangeMarketplaces: (v: CodexMarketplace[]) => void
  onChangePlugins: (v: string[]) => void
}

export function CodexPluginsSection({
  marketplaces,
  plugins,
  onChangeMarketplaces,
  onChangePlugins,
}: Props) {
  const [discovered, setDiscovered] = useState<CodexDiscoveredMarketplace[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [newName, setNewName] = useState('')
  const [newSource, setNewSource] = useState('')
  const [newType, setNewType] = useState<'local' | 'git'>('local')

  const discover = async () => {
    setBusy(true)
    try {
      setDiscovered(await workspaceApi.discoverCodexMarketplaces())
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Marketplace taraması başarısız')
    } finally {
      setBusy(false)
    }
  }

  const importOne = async (d: CodexDiscoveredMarketplace) => {
    setBusy(true)
    try {
      const res = await workspaceApi.importCodexMarketplace({
        root: d.root,
        name: d.suggestedName,
      })
      // Replace an entry of the same name rather than adding a duplicate: a
      // re-import is how the user refreshes a copy after updating Codex.
      onChangeMarketplaces([
        ...marketplaces.filter((m) => m.name !== res.marketplace.name),
        res.marketplace,
      ])
      toast.success(
        `${res.marketplace.name} kopyalandı — ${res.availablePlugins.length} plugin kullanılabilir`,
      )
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'İçe aktarma başarısız')
    } finally {
      setBusy(false)
    }
  }

  const addManual = () => {
    const name = newName.trim()
    const source = newSource.trim()
    if (!name || !source) return
    onChangeMarketplaces([
      ...marketplaces.filter((m) => m.name !== name),
      { name, source, sourceType: newType },
    ])
    setNewName('')
    setNewSource('')
  }

  const removeMarketplace = (name: string) => {
    onChangeMarketplaces(marketplaces.filter((m) => m.name !== name))
    // Selectors pointing at a removed marketplace would resolve to nothing, so
    // they go with it.
    onChangePlugins(plugins.filter((p) => !p.endsWith('@' + name)))
  }

  const togglePlugin = (selector: string) => {
    onChangePlugins(
      plugins.includes(selector) ? plugins.filter((p) => p !== selector) : [...plugins, selector],
    )
  }

  return (
    <div className="flex flex-col gap-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] p-3">
      {/* Configured marketplaces */}
      <div className="flex flex-col gap-1.5">
        <div className="text-xs font-medium text-[var(--color-text)]">Marketplace'ler</div>
        {marketplaces.length === 0 ? (
          <div className="text-xs text-[var(--color-text-dim)]">
            Henüz marketplace yok. Aşağıdan makinendekileri tara veya kendi yerel/Git kaynağını
            ekle.
          </div>
        ) : (
          marketplaces.map((m) => (
            <div
              key={m.name}
              className="flex items-center gap-2 rounded-md border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-1.5"
            >
              <div className="min-w-0 flex-1">
                <div className="text-xs font-medium text-[var(--color-text)]">{m.name}</div>
                <div className="truncate font-mono text-[10px] text-[var(--color-text-dim)]">
                  {m.sourceType} · {m.source}
                </div>
              </div>
              <button
                type="button"
                onClick={() => removeMarketplace(m.name)}
                aria-label={`${m.name} marketplace'ini kaldır`}
                className="shrink-0 rounded p-1 text-[var(--color-text-dim)] hover:text-[var(--color-danger)]"
              >
                <Trash2 size={14} />
              </button>
            </div>
          ))
        )}
      </div>

      {/* Enabled plugin selectors */}
      {plugins.length > 0 && (
        <div className="flex flex-col gap-1.5">
          <div className="text-xs font-medium text-[var(--color-text)]">Etkin plugin'ler</div>
          <div className="flex flex-wrap gap-1.5">
            {plugins.map((p) => (
              <button
                key={p}
                type="button"
                onClick={() => togglePlugin(p)}
                className="rounded-full border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-0.5 font-mono text-[10px] text-[var(--color-text)]"
              >
                {p} ✕
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Manual source */}
      <div className="flex flex-col gap-1.5 border-t border-[var(--color-border)] pt-2">
        <div className="text-xs font-medium text-[var(--color-text)]">Kendi kaynağını ekle</div>
        <div className="flex flex-wrap items-center gap-1.5">
          <input
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            placeholder="ad (harf, rakam, _ -)"
            className={`${inputCls} w-40`}
          />
          <select
            value={newType}
            onChange={(e) => setNewType(e.target.value as 'local' | 'git')}
            className={`${inputCls} w-24`}
          >
            <option value="local">local</option>
            <option value="git">git</option>
          </select>
          <input
            value={newSource}
            onChange={(e) => setNewSource(e.target.value)}
            placeholder={newType === 'local' ? 'C:\\...\\marketplace' : 'owner/repo veya URL'}
            className={`${inputCls} min-w-0 flex-1`}
          />
          <Button onClick={addManual} disabled={!newName.trim() || !newSource.trim()}>
            <Plus size={14} /> Ekle
          </Button>
        </div>
      </div>

      {/* Discovery + import */}
      <div className="flex flex-col gap-1.5 border-t border-[var(--color-border)] pt-2">
        <div className="flex items-center justify-between gap-2">
          <div className="text-xs font-medium text-[var(--color-text)]">
            Makinendeki Codex plugin'leri
          </div>
          <Button onClick={discover} disabled={busy}>
            <RefreshCw size={14} /> {busy ? 'Taranıyor…' : 'Tara'}
          </Button>
        </div>
        <div className="text-[11px] text-[var(--color-text-dim)]">
          Bulunan marketplace bu workspace'e <strong>kopyalanır</strong> ve yeni bir adla kaydedilir
          — Codex kendi adlarını (örn. <code>openai-bundled</code>) başka bir kaynaktan eklemeyi
          reddeder. Bundled plugin'ler OpenAI'ın tescilli dosyalarıdır; kopyalama yalnız senin
          isteğinle yapılır.
        </div>
        {discovered?.length === 0 && (
          <div className="text-xs text-[var(--color-text-dim)]">
            Bu makinede marketplace bulunamadı.
          </div>
        )}
        {discovered?.map((d) => (
          <div
            key={d.root}
            className="flex items-center gap-2 rounded-md border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-1.5"
          >
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-1.5">
                <span className="text-xs font-medium text-[var(--color-text)]">{d.name}</span>
                {d.reserved && (
                  <span className="rounded bg-[color-mix(in_srgb,var(--color-warning)_15%,transparent)] px-1 py-0.5 text-[10px] text-[var(--color-warning)]">
                    ayrılmış ad → {d.suggestedName}
                  </span>
                )}
              </div>
              <div className="truncate text-[10px] text-[var(--color-text-dim)]">
                {d.plugins.length} plugin · {d.root}
              </div>
            </div>
            <Button onClick={() => importOne(d)} disabled={busy}>
              <Download size={14} /> İçe aktar
            </Button>
          </div>
        ))}
      </div>
    </div>
  )
}
