// zvec-grep integration callout, anchored under the `zg` row of the External
// Tools panel. It explains what the workspace capability does and adds or removes
// the MCP server in one click. Same shape as the codebase-memory callout in
// ExternalToolsPanel, kept in its own file so that panel does not grow a second
// inline block.
import { useState } from 'react'
import { api } from '@/api'
import type { ExternalToolStatus, MCPServer } from '@/types'
import {
  ZVEC_GREP_SERVER_ARGS,
  ZVEC_GREP_SERVER_NAME,
  isZvecGrepServer,
} from '@/shared/lib/zvecGrep'
import { Trans, useTranslation } from 'react-i18next'
import { InfoPopover } from '@/shared/components/InfoPopover'

interface Props {
  tool: ExternalToolStatus
  servers: MCPServer[]
  onServersChanged: () => Promise<unknown> | void
  onError: (msg: string) => void
}

export function ZvecGrepCallout({ tool, servers, onServersChanged, onError }: Props) {
  const { t } = useTranslation('settings')
  const [busy, setBusy] = useState(false)
  // Matched by the backend's own rule, so a server the user added by hand under
  // another name still reads as wired.
  const server = servers.find(isZvecGrepServer)

  const toggle = async () => {
    setBusy(true)
    try {
      if (server) await api.deleteMCPServer(server.id)
      else
        await api.createMCPServer({
          name: ZVEC_GREP_SERVER_NAME,
          transport: 'stdio',
          command: tool.path ?? tool.name,
          args: [...ZVEC_GREP_SERVER_ARGS],
        })
      await onServersChanged()
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div
      data-testid="zvec-callout"
      className="rounded-lg border border-[color-mix(in_srgb,var(--color-accent)_30%,transparent)] bg-[color-mix(in_srgb,var(--color-accent)_6%,transparent)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]"
    >
      <span className="flex items-center gap-1 font-medium text-[var(--color-text)]">
        🔎 {t('zvec.title')}
        <InfoPopover
          text={
            <Trans
              i18nKey="zvec.description"
              ns="settings"
              components={{
                code: <code />,
                strong: <span className="font-medium text-[var(--color-text)]" />,
              }}
            />
          }
        />
      </span>
      <div className="mt-2 flex items-center gap-2 border-t border-[color-mix(in_srgb,var(--color-accent)_20%,transparent)] pt-2">
        <button
          type="button"
          data-testid="zvec-mcp-toggle"
          disabled={busy || !tool.found}
          onClick={toggle}
          className={`rounded px-2.5 py-1 text-xs font-medium disabled:opacity-50 ${
            server
              ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
              : 'bg-[var(--color-accent)] text-[var(--color-on-accent)] hover:opacity-90'
          }`}
          title={
            !tool.found
              ? t('zvec.installFirst')
              : server
                ? t('zvec.removeTitle')
                : t('zvec.addTitle')
          }
        >
          {busy ? '…' : server ? t('zvec.remove') : t('zvec.add')}
        </button>
        <span className="text-[11px] text-[var(--color-text-dim)]">
          {!tool.found
            ? t('zvec.notFound')
            : server
              ? t('zvec.connected', { name: server.name })
              : t('zvec.notConnected')}
        </span>
      </div>
    </div>
  )
}
