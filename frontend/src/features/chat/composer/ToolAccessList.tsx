// Presentational lists for the composer's tool inspector: grouped tool rows and
// the MCP server inventory. Read-only — nothing here mutates configuration.
import type { ToolAccessEntry, ToolAccessServer } from '@/types'
import { visibilityMeta } from '@/features/tools/toolMeta'
import { groupTools } from './toolAccessGroups'

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

export function ToolGroupList({ tools }: { tools: ToolAccessEntry[] }) {
  const groups = groupTools(tools)
  if (groups.length === 0) {
    return <div className="px-1 py-3 text-xs text-[var(--color-text-dim)]">Eşleşen araç yok.</div>
  }
  return (
    <div className="flex flex-col gap-3">
      {groups.map((g) => (
        <div key={g.key}>
          <div className="mb-1 flex items-center gap-1.5 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
            <span>{g.source === 'mcp' ? '🔌' : '🧩'}</span>
            <span>{g.label}</span>
            <span className="opacity-60">({g.tools.length})</span>
          </div>
          <div className="flex flex-col">
            {g.tools.map((t) => (
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
            ))}
          </div>
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
}: {
  servers: ToolAccessServer[]
  poolIdleSec: number
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
        <div
          key={s.id}
          className="flex items-center gap-2 rounded-lg px-1 py-1.5 hover:bg-[var(--color-surface-2)]"
        >
          <span
            title={s.enabled ? 'Etkin' : 'Devre dışı'}
            className="shrink-0"
            style={{ color: s.enabled ? 'var(--color-success)' : 'var(--color-text-dim)' }}
          >
            ●
          </span>
          <span className="shrink-0 text-xs font-medium text-[var(--color-text)]">{s.name}</span>
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
            title="Bu sunucudan her tur şeması gönderilen araç sayısı"
            className="shrink-0 text-[10px] text-[var(--color-text-dim)]"
          >
            aktif {s.eagerCount}
          </span>
          <span
            title="Bu sunucudan talep üzerine (activate_tools) açılabilen araç sayısı"
            className="shrink-0 text-[10px] text-[var(--color-text-dim)]"
          >
            hazır {s.lazyCount}
          </span>
        </div>
      ))}
    </div>
  )
}
