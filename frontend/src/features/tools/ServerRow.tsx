import { Link2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { MCPPoolStats, MCPServer, ToolVisibility } from '@/types'
import { parseArgs, VISIBILITY_TIERS } from './toolMeta'

const BADGE =
  'shrink-0 rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-xs text-[var(--color-text-dim)]'
const ACTION = 'rounded bg-[var(--color-surface-2)] px-2 py-1 text-xs hover:opacity-90'
// Complete class sets per state: the edit button's fill differs while its row is
// being edited, and two bg-*/text-* utilities on one element are resolved by
// stylesheet order, not class order.
const ACTION_ACTIVE =
  'rounded bg-[var(--color-accent)] px-2 py-1 text-xs text-[var(--color-on-accent)] hover:opacity-90'

interface Props {
  server: MCPServer
  // How many of this server's MCP tools exist — drives the "all tools →" row.
  toolCount: number
  poolStats: MCPPoolStats | null
  testing: boolean
  testResult?: string
  editing: boolean
  onTest: () => void
  onCopy: () => void
  onToggle: () => void
  onEdit: () => void
  onRemove: () => void
  onVisibility: (tier: ToolVisibility) => void
}

// ServerRow is one configured MCP server in the server list.
//
// Layout: from `sm` up the identity (name + badges over the command/URL) sits
// left and the actions right, on one line. Below `sm` it stacks — name and
// badges, then the command/URL, then the actions in a wrapping row — so a narrow
// phone never squeezes the name into a column of single letters or lets the
// actions overlap it. The name truncates as a whole and keeps its full text in
// the tooltip.
export function ServerRow({
  server: s,
  toolCount,
  poolStats,
  testing,
  testResult,
  editing,
  onTest,
  onCopy,
  onToggle,
  onEdit,
  onRemove,
  onVisibility,
}: Props) {
  const { t: tr } = useTranslation('tools')
  // Live-connection / reaper indicator from the pool snapshot.
  const live = poolStats?.servers.find((x) => x.server === s.name)
  const idleMin = poolStats ? Math.round(poolStats.idleSec / 60) : 0
  const target =
    s.transport === 'stdio' ? `${s.command} ${parseArgs(s.args).join(' ')}`.trim() : s.url

  return (
    <div
      data-testid="mcp-server-row"
      className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
    >
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0">
          <div className="flex min-w-0 items-center gap-2">
            <span
              data-testid="mcp-server-name"
              className="min-w-0 truncate font-medium"
              title={s.name}
            >
              {s.name}
            </span>
            <span className={BADGE}>{s.transport}</span>
            {s.scope === 'scoped' && (
              <span
                data-testid="mcp-server-scope-badge"
                className="shrink-0 rounded bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-xs text-[var(--color-accent)]"
                title={tr('server.scopedBadgeHint')}
              >
                {tr('server.scopedBadge')}
              </span>
            )}
            {live && live.live > 0 && (
              <span
                data-testid="mcp-server-live-badge"
                className={`${BADGE} inline-flex items-center gap-1`}
                title={
                  live.scoped
                    ? tr('server.liveScopedHint', {
                        live: live.live,
                        total: live.total,
                        minutes: idleMin,
                      })
                    : tr('server.liveSharedHint', { live: live.live })
                }
              >
                <Link2 size={12} className="shrink-0" />
                {live.live}
                {live.scoped ? tr('server.sessionSuffix') : ''}
              </span>
            )}
            {!s.enabled && (
              <span className="shrink-0 text-xs text-[var(--color-text-dim)]">
                {tr('status.disabledParenthetical')}
              </span>
            )}
          </div>
          <div className="truncate text-xs text-[var(--color-text-dim)]" title={target}>
            {target}
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2 sm:flex-shrink-0 sm:flex-nowrap">
          <button
            data-testid="mcp-server-test"
            data-server-id={s.id}
            onClick={onTest}
            disabled={testing}
            className={ACTION}
          >
            {tr('actions.test')}
          </button>
          <button
            data-testid="mcp-server-copy"
            data-server-id={s.id}
            onClick={onCopy}
            title={tr('server.copyJsonHint')}
            className={ACTION}
          >
            {tr('server.json')}
          </button>
          <button
            data-testid="mcp-server-toggle"
            data-server-id={s.id}
            role="switch"
            aria-checked={s.enabled}
            onClick={onToggle}
            title={s.enabled ? tr('actions.disable') : tr('actions.enable')}
            className={
              s.enabled
                ? 'relative h-5 w-9 flex-shrink-0 rounded-full bg-[var(--color-accent)] transition'
                : 'relative h-5 w-9 flex-shrink-0 rounded-full bg-[var(--color-surface-2)] transition'
            }
          >
            <span
              className={
                s.enabled
                  ? 'absolute left-4 top-0.5 h-4 w-4 rounded-full bg-[var(--color-text)] transition-all'
                  : 'absolute left-0.5 top-0.5 h-4 w-4 rounded-full bg-[var(--color-text)] transition-all'
              }
            />
          </button>
          <button
            data-testid="mcp-server-edit"
            data-server-id={s.id}
            onClick={onEdit}
            title={tr('server.editHint')}
            className={editing ? ACTION_ACTIVE : ACTION}
          >
            {tr('actions.edit')}
          </button>
          <button
            data-testid="mcp-server-delete"
            data-server-id={s.id}
            onClick={onRemove}
            className="rounded px-2 py-1 text-xs text-[var(--color-danger)] hover:bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)]"
          >
            {tr('actions.delete')}
          </button>
        </div>
      </div>
      {toolCount > 0 && (
        <div className="mt-2 flex flex-wrap items-center gap-2 border-t border-[var(--color-border)] pt-2">
          <span className="text-xs text-[var(--color-text-dim)]">
            {tr('server.allToolsArrow', { count: toolCount })}
          </span>
          {VISIBILITY_TIERS.map((tier) => (
            <button
              key={tier.value}
              data-testid="mcp-server-visibility-all"
              data-server-id={s.id}
              data-tier={tier.value}
              onClick={() => onVisibility(tier.value)}
              title={tier.hint}
              className={ACTION}
            >
              {tier.label}
            </button>
          ))}
        </div>
      )}
      {testResult && (
        <div className="mt-2 break-words text-xs text-[var(--color-text-dim)]">{testResult}</div>
      )}
    </div>
  )
}
