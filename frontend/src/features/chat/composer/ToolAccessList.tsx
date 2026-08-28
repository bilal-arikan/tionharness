// Presentational lists for the composer's tool inspector: grouped tool rows and
// the MCP server inventory. Read-only — nothing here mutates configuration.
import { useState } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import type { ToolAccessEntry, ToolAccessServer, ToolAccessServerStatus } from '@/types'
import { visibilityMeta } from '@/features/tools/toolMeta'
import { filterTools, sortTools, toolsForServer } from './toolAccessGroups'

// TierBadge shows the tool's effective visibility tier (Tam / Özet / İsim / Gizli)
// — i.e. how much of its schema reaches the model, which is the real cost driver.
function TierBadge({ visibility }: { visibility: ToolAccessEntry['visibility'] }) {
  const meta = visibilityMeta(visibility)
  return (
    <span
      title={meta.hint}
      className="shrink-0 rounded-md border px-1 text-[10px] leading-4"
      style={{ borderColor: meta.color, color: meta.labelColor ?? meta.color }}
    >
      {meta.label}
    </span>
  )
}

// STATUS_META answers "is this server in the agent's context, and if not why?".
// The label is the verdict; the hint explains which knob changes it.
//
// Note the wording of 'hidden-only': the tools are out of the CATALOG, not out of
// context altogether — the prompt still carries a one-line pointer telling the
// agent they exist and how to find them. Calling that "bağlam dışı" would read as
// "unavailable", which is wrong.
const STATUS_META: Record<ToolAccessServerStatus, { label: string; hint: string; color: string }> =
  {
    'in-context': {
      label: 'bağlamda',
      hint: 'Bu sunucunun araçları ajanın promptunda — çağırabilir.',
      color: 'var(--color-success)',
    },
    'hidden-only': {
      label: 'katalog dışı',
      hint: 'Araçları "Gizli" tier\'da: katalogda tek tek listelenmez (bağlamda yalnız "N araç daha var, tool_search ile bul" notu durur), aktive edilince normal çağrılır.',
      color: 'var(--color-warning)',
    },
    disabled: {
      label: 'kapalı',
      hint: "Sunucu bu workspace'te devre dışı — Araçlar ekranından açılabilir.",
      color: 'var(--color-text-dim)',
    },
    'agent-mcp-off': {
      label: 'ajanda MCP kapalı',
      hint: 'Sunucu etkin ama bu ajanın MCP anahtarı kapalı — hiçbir MCP aracı sunulmuyor.',
      color: 'var(--color-warning)',
    },
    'no-tools': {
      label: 'araç yok',
      hint: 'Sunucu etkin ama araç gelmiyor: bağlantı kurulamamış ya da tüm araçları yasaklı olabilir.',
      color: 'var(--color-danger)',
    },
  }

// Rough per-line cost of a catalogued lazy tool ("- `name` — one-line summary")
// in the load-on-demand block. Only used to put an order of magnitude on what the
// hidden tier saves; not a billing figure.
const CATALOG_LINE_TOKENS = 20

function hiddenHint(count: number): string {
  return `Katalogda tek tek listelenmeyen araç sayısı — bağlamda yalnız "tool_search ile bulunabilir" notu durur (≈${count * CATALOG_LINE_TOKENS} token tasarruf, her turda).`
}

// SourceBadge names where a tool comes from — a built-in category or an MCP
// server — as a small secondary tag on the row. It is NOT a grouping key: each
// tab is one flat list where built-in and MCP tools mix, so this badge is the
// only place source still shows.
function SourceBadge({ t }: { t: ToolAccessEntry }) {
  const label = t.source === 'mcp' ? t.server || 'MCP' : (t.category ?? 'builtin')
  return (
    <span className="shrink-0 truncate text-[10px] text-[var(--color-text-dim)] opacity-70">
      {t.source === 'mcp' ? '🔌' : '🧩'} {label}
    </span>
  )
}

// ToolList renders ONE tab's tools as a single flat, alphabetical list. The tab
// itself is the context split (in context vs. on demand), so there is no second
// level of grouping here and no source split either — built-in and MCP rows sit
// side by side, told apart by their badges.
//
// `empty` is the message for a genuinely empty tab; a query that matches nothing
// says so instead, because the two mean very different things.
export function ToolList({
  tools,
  query,
  empty,
}: {
  tools: ToolAccessEntry[]
  query: string
  empty: string
}) {
  const shown = sortTools(filterTools(tools, query))
  if (shown.length === 0) {
    return (
      <div className="px-1 py-3 text-xs text-[var(--color-text-dim)]">
        {tools.length === 0 ? empty : 'Aramayla eşleşen araç yok.'}
      </div>
    )
  }
  return (
    <div className="flex flex-col">
      {shown.map((t) => (
        <div
          key={t.name}
          data-testid="tool-access-row"
          className="flex items-start gap-2 rounded-lg px-1 py-1 hover:bg-[var(--color-surface-2)]"
        >
          <code className="shrink-0 text-xs text-[var(--color-text)]">{t.label}</code>
          <TierBadge visibility={t.visibility} />
          <SourceBadge t={t} />
          <span className="min-w-0 flex-1 truncate text-xs text-[var(--color-text-dim)]">
            {t.description}
          </span>
        </div>
      ))}
    </div>
  )
}

// ServerList shows every configured MCP server — enabled or not — with how many
// of its tools this agent gets eagerly vs on demand, and its live connections.
// A disabled server contributes zero tools; it is listed so the user can see what
// is available to switch on from the Tools screen.
export function ServerList({
  servers,
  poolIdleSec,
  tools,
  query,
}: {
  servers: ToolAccessServer[]
  poolIdleSec: number
  // Every tool entry the panel already fetched, so an expanded server row can
  // show its own tools without a second request.
  tools: { eager: ToolAccessEntry[]; lazy: ToolAccessEntry[] }
  query: string
}) {
  if (servers.length === 0) {
    return (
      <div className="px-1 py-3 text-xs text-[var(--color-text-dim)]">
        Bu workspace'te tanımlı MCP sunucusu yok.
      </div>
    )
  }
  return (
    <div className="flex flex-col gap-1">
      {servers.map((s) => (
        <ServerRow
          key={s.id}
          server={s}
          poolIdleSec={poolIdleSec}
          tools={toolsForServer(tools, s.name)}
          query={query}
        />
      ))}
    </div>
  )
}

// ServerRow renders one server line and, when expanded, that server's own tools.
// Expansion is local state: the inspector is read-only and short-lived, so there
// is nothing to persist.
function ServerRow({
  server: s,
  poolIdleSec,
  tools,
  query,
}: {
  server: ToolAccessServer
  poolIdleSec: number
  tools: ToolAccessEntry[]
  query: string
}) {
  const [open, setOpen] = useState(false)
  const st = STATUS_META[s.status]
  const shown = filterTools(tools, query)
  return (
    <div data-testid="tool-access-server-row" data-server={s.name}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        data-testid="tool-access-server-toggle"
        className="flex w-full items-center gap-2 rounded-lg px-1 py-1.5 text-left hover:bg-[var(--color-surface-2)]"
      >
        {open ? (
          <ChevronDown size={12} className="shrink-0 text-[var(--color-text-dim)]" />
        ) : (
          <ChevronRight size={12} className="shrink-0 text-[var(--color-text-dim)]" />
        )}
        <span title={st.hint} className="shrink-0" style={{ color: st.color }}>
          ●
        </span>
        <span className="shrink-0 text-xs font-medium text-[var(--color-text)]">{s.name}</span>
        <span
          title={st.hint}
          className="shrink-0 rounded-md border px-1 text-[10px] leading-4"
          style={{ borderColor: st.color, color: st.color }}
        >
          {st.label}
        </span>
        <span className="shrink-0 text-[10px] text-[var(--color-text-dim)]">
          {s.transport}
          {s.scope === 'scoped' ? ' · oturum-özel' : ''}
        </span>
        <div className="flex-1" />
        {s.live > 0 && (
          <span
            title={
              s.scope === 'scoped' && poolIdleSec > 0
                ? `${s.live}/${s.total} canlı bağlantı — ${poolIdleSec}sn boşta kalırsa kapanır`
                : `${s.live}/${s.total} canlı bağlantı`
            }
            className="shrink-0 text-[10px] text-[var(--color-success)]"
          >
            🔗 {s.live}
          </span>
        )}
        <span
          title="Bu sunucudan her tur TAM şeması gönderilen araç sayısı"
          className="shrink-0 text-[10px] text-[var(--color-text-dim)]"
        >
          aktif {s.eagerCount}
        </span>
        <span
          title="Katalogda isim/özet olarak duran, activate_tools ile açılabilen araç sayısı"
          className="shrink-0 text-[10px] text-[var(--color-text-dim)]"
        >
          katalog {s.lazyCount}
        </span>
        {s.hiddenCount > 0 && (
          <span
            title={hiddenHint(s.hiddenCount)}
            className="shrink-0 text-[10px] text-[var(--color-text-dim)] opacity-70"
          >
            gizli {s.hiddenCount}
          </span>
        )}
      </button>
      {open && (
        <div className="mb-1 ml-5 flex flex-col border-l border-[var(--color-border)] pl-2">
          {shown.length === 0 ? (
            <div className="px-1 py-1.5 text-[11px] leading-4 text-[var(--color-text-dim)]">
              {tools.length === 0 ? st.hint : 'Aramayla eşleşen araç yok.'}
            </div>
          ) : (
            shown.map((t) => (
              <div
                key={t.name}
                className="flex items-start gap-2 rounded-lg px-1 py-1 hover:bg-[var(--color-surface-2)]"
              >
                <code className="shrink-0 text-xs text-[var(--color-text)]">{t.label}</code>
                <TierBadge visibility={t.visibility} />
                <span className="min-w-0 flex-1 truncate text-xs text-[var(--color-text-dim)]">
                  {t.description}
                </span>
              </div>
            ))
          )}
        </div>
      )}
    </div>
  )
}
