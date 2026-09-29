import { useTranslation } from 'react-i18next'
import type { MCPDisconnectNotice } from './mcpDisconnects'

// Visible notice for an MCP server that dropped mid-session.
//
// Only UNEXPECTED deaths reach this component (the pool filters out every
// shutdown TionHarness itself initiated), so the wording commits to a failure
// rather than hedging. A scoped connection's loss is worded differently on
// purpose: it affects one session, while the server may still be serving others.
export function MCPDisconnectNotices({
  notices,
  onDismiss,
}: {
  notices: MCPDisconnectNotice[]
  onDismiss: (server: string) => void
}) {
  const { t } = useTranslation('tools')
  if (notices.length === 0) return null
  return (
    <div className="mb-3 flex flex-col gap-2" data-testid="mcp-disconnect-notices">
      {notices.map((n) => (
        <div
          key={n.server}
          data-testid="mcp-disconnect-notice"
          data-server={n.server}
          className="flex items-start gap-2 rounded border border-[var(--color-danger)] bg-[var(--color-surface-2)] px-3 py-2 text-sm"
        >
          <span aria-hidden="true">{t('mcpDisconnect.warningIcon')}</span>
          <div className="min-w-0 flex-1">
            <div className="font-medium">
              {n.scoped
                ? t('mcpDisconnect.titleScoped', { server: n.server })
                : t('mcpDisconnect.title', { server: n.server })}
            </div>
            {n.error && (
              <div className="truncate text-xs text-[var(--color-text-dim)]" title={n.error}>
                {n.error}
              </div>
            )}
            {n.pendingCalls > 0 && (
              <div className="text-xs text-[var(--color-text-dim)]">
                {t('mcpDisconnect.pendingCalls', { count: n.pendingCalls })}
              </div>
            )}
            <div className="text-xs text-[var(--color-text-dim)]">{t('mcpDisconnect.hint')}</div>
          </div>
          <button
            data-testid="mcp-disconnect-dismiss"
            onClick={() => onDismiss(n.server)}
            className="rounded bg-[var(--color-surface-1)] px-2 py-1 text-xs hover:opacity-90"
          >
            {t('mcpDisconnect.dismiss')}
          </button>
        </div>
      ))}
    </div>
  )
}
