import { useEffect, useRef, useState } from 'react'
import { Wrench, X } from 'lucide-react'
import { api } from '@/api'
import type { AgentToolAccess } from '@/types'
import { filterTools } from './toolAccessGroups'
import { ServerList, ToolGroupList } from './ToolAccessList'

interface Props {
  // The composer's selected agent — tool access is per-agent (workspace catalog
  // minus the agent's own overrides), so the panel follows that selection.
  agentId: string
  onClose: () => void
}

type Tab = 'eager' | 'lazy' | 'servers'

// ToolAccessPanel is a READ-ONLY inspector: which tools the selected agent can
// use right now, and what the MCP gateway has open vs what it could open.
//
// The split it shows is the one that actually costs tokens:
//   - "Aktif" (eager): full schemas are shipped with every turn.
//   - "Talep üzerine" (lazy): only name/description sit in the load-on-demand
//     catalog; the agent pulls the schema in mid-turn via tool_search /
//     activate_tools. These are the tools it CAN activate but has not.
//   - "MCP": every configured server, enabled or not, with its live connections.
//
// Nothing here changes configuration — that lives on the Tools screen.
export function ToolAccessPanel({ agentId, onClose }: Props) {
  const [data, setData] = useState<AgentToolAccess | null>(null)
  const [error, setError] = useState('')
  const [tab, setTab] = useState<Tab>('eager')
  const [query, setQuery] = useState('')
  const rootRef = useRef<HTMLDivElement>(null)

  // Fetch once per mount. The parent keys this component by agentId, so switching
  // the target agent remounts it with clean state instead of resetting in-effect.
  useEffect(() => {
    let alive = true
    api
      .agentToolAccess(agentId)
      .then((d) => alive && setData(d))
      .catch((e: unknown) => alive && setError((e as Error).message))
    return () => {
      alive = false
    }
  }, [agentId])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  // Click-outside closes. The toggle button lives OUTSIDE this element (it is a
  // toolbar sibling, not a wrapper), so a click on it must be ignored here —
  // otherwise mousedown would close the panel and the button's own click would
  // immediately reopen it, making the button look dead.
  useEffect(() => {
    const onDown = (e: MouseEvent) => {
      const target = e.target as HTMLElement | null
      if (rootRef.current?.contains(target)) return
      if (target?.closest('[data-tool-access-toggle]')) return
      onClose()
    }
    document.addEventListener('mousedown', onDown)
    return () => document.removeEventListener('mousedown', onDown)
  }, [onClose])

  // Lazy tools that are not even catalogued (tier 'hidden') — the self-management
  // suite and friends. Called out explicitly so "not listed" never reads as
  // "unavailable": the prompt still carries a pointer to them.
  const hiddenCount = data?.lazy.filter((t) => !t.inContext).length ?? 0

  const tabs: { id: Tab; label: string; count?: number }[] = [
    { id: 'eager', label: 'Aktif', count: data?.eager.length },
    { id: 'lazy', label: 'Talep üzerine', count: data?.lazy.length },
    { id: 'servers', label: 'MCP', count: data?.servers.length },
  ]

  return (
    <div
      ref={rootRef}
      data-testid="tool-access-panel"
      className="absolute bottom-full left-3 z-20 mb-2 w-[min(34rem,calc(100vw-2rem))] rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-3 shadow-2xl md:left-6"
    >
      <div className="mb-2 flex items-center gap-2">
        <Wrench size={16} className="text-[var(--color-accent)]" />
        <span className="text-sm font-medium">Araçlar</span>
        <span className="truncate text-xs text-[var(--color-text-dim)]">
          {data ? `${data.agentName} · salt bilgi` : 'salt bilgi'}
        </span>
        <div className="flex-1" />
        <button
          type="button"
          onClick={onClose}
          title="Kapat"
          aria-label="Kapat"
          data-testid="tool-access-close"
          className="text-[var(--color-text-dim)] transition hover:text-[var(--color-text)]"
        >
          <X size={16} />
        </button>
      </div>

      {error && (
        <div className="rounded-xl border border-[var(--color-danger)] px-2.5 py-2 text-xs text-[var(--color-danger)]">
          {error}
        </div>
      )}
      {!data && !error && (
        <div className="px-1 py-3 text-xs text-[var(--color-text-dim)]">Yükleniyor…</div>
      )}

      {data && (
        <>
          <div className="mb-2 flex items-center gap-1.5">
            {tabs.map((t) => (
              <button
                key={t.id}
                type="button"
                onClick={() => setTab(t.id)}
                data-testid={`tool-access-tab-${t.id}`}
                className={`rounded-lg px-2 py-1 text-xs transition ${
                  tab === t.id
                    ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]'
                    : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'
                }`}
              >
                {t.label}
                {t.count !== undefined && <span className="ml-1 opacity-60">{t.count}</span>}
              </button>
            ))}
            <div className="flex-1" />
            {tab !== 'servers' && (
              <input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Ara…"
                aria-label="Araç ara"
                data-testid="tool-access-search"
                className="w-28 rounded-lg border border-[var(--color-border)] bg-transparent px-2 py-1 text-xs outline-none focus:border-[var(--color-accent)] placeholder:text-[var(--color-text-dim)]"
              />
            )}
          </div>

          <p className="mb-2 text-[11px] leading-4 text-[var(--color-text-dim)]">
            {tab === 'eager' &&
              'Bu araçların tam şeması her turda modele gönderilir — anında çağırabilir.'}
            {tab === 'lazy' &&
              `Bu araçların şeması turda gönderilmez; ajan gerektiğinde tool_search / activate_tools ile yükler. ${hiddenCount} tanesi "Gizli" tier'da: katalogda tek tek listelenmez, bağlamda yalnız "bunlar da var, tool_search ile bul" notu durur — bilinmez değil, ucuzdur.`}
            {tab === 'servers' &&
              'Workspace’teki (özel dâhil) tüm MCP sunucuları ve araçlarının bağlama girip girmediği: yeşil = promptta, sarı/gri = değil (sebebi rozetin üstünde). 🔗 = şu an açık canlı bağlantı. Ayarlar Araçlar ekranından değiştirilir.'}
          </p>

          <div className="max-h-[22rem] overflow-y-auto pr-1">
            {tab === 'eager' && <ToolGroupList tools={filterTools(data.eager, query)} />}
            {tab === 'lazy' && <ToolGroupList tools={filterTools(data.lazy, query)} />}
            {tab === 'servers' && (
              <ServerList servers={data.servers} poolIdleSec={data.poolIdleSec} />
            )}
          </div>

          {data.blocked.length > 0 && (
            <div
              title={data.blocked.join(', ')}
              className="mt-2 truncate border-t border-[var(--color-border)] pt-2 text-[11px] text-[var(--color-text-dim)]"
            >
              Yasaklı ({data.blocked.length}): {data.blocked.join(', ')}
            </div>
          )}
          {!data.mcpEnabled && (
            <div className="mt-2 text-[11px] text-[var(--color-warning,#d97706)]">
              Bu ajanda MCP kapalı — MCP sunucularının araçları sunulmuyor.
            </div>
          )}
        </>
      )}
    </div>
  )
}
