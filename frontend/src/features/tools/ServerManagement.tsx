import { useState } from 'react'
import type {
  MCPServer,
  MCPTransport,
  MCPPoolStats,
  ToolVisibility,
  WorkspaceTool,
  ImportableMCPServer,
} from '@/types'
import { parseArgs, serverToImportJson, toolSource, toolServer } from './toolMeta'
import { ServerRow } from './ServerRow'
import { EmptyState, InfoPopover, ModalOverlay, PaneHeader, toast } from '@/shared/components'
import { useTranslation } from 'react-i18next'

// ServerManagement is the MCP server list + add form, shown when no tool is
// selected. (Extracted so the right pane stays readable.)
export function ServerManagement(props: {
  servers: MCPServer[]
  tools: WorkspaceTool[]
  onServerVisibility: (server: string, tier: ToolVisibility) => void
  testing: string | null
  testResult: Record<string, string>
  name: string
  setName: (v: string) => void
  transport: MCPTransport
  setTransport: (v: MCPTransport) => void
  command: string
  setCommand: (v: string) => void
  argsText: string
  setArgsText: (v: string) => void
  url: string
  setUrl: (v: string) => void
  headersText: string
  setHeadersText: (v: string) => void
  scope: 'shared' | 'scoped'
  setScope: (v: 'shared' | 'scoped') => void
  editingId: string | null
  onEdit: (s: MCPServer) => void
  onCancelEdit: () => void
  poolStats: MCPPoolStats | null
  onAdd: () => void
  onToggle: (s: MCPServer) => void
  onTest: (s: MCPServer) => void
  onRemove: (s: MCPServer) => void
  importText: string
  setImportText: (v: string) => void
  importing: boolean
  importMsg: string
  onImport: () => void
  importable: ImportableMCPServer[]
  addingImportable: string | null
  onLoadImportable: () => void
  onAddImportable: (item: ImportableMCPServer) => void
}) {
  const { t: tr } = useTranslation('tools')
  const {
    servers,
    tools,
    onServerVisibility,
    testing,
    testResult,
    name,
    setName,
    transport,
    setTransport,
    command,
    setCommand,
    argsText,
    setArgsText,
    url,
    setUrl,
    headersText,
    setHeadersText,
    scope,
    setScope,
    editingId,
    onEdit,
    onCancelEdit,
    poolStats,
    onAdd,
    onToggle,
    onTest,
    onRemove,
    importText,
    setImportText,
    importing,
    importMsg,
    onImport,
    importable,
    addingImportable,
    onLoadImportable,
    onAddImportable,
  } = props
  // "Diğer workspace'lerden ekle" popup: opens on demand and loads the candidates.
  const [showImportable, setShowImportable] = useState(false)
  const openImportable = () => {
    setShowImportable(true)
    onLoadImportable()
  }
  const copyServer = async (s: MCPServer) => {
    const json = serverToImportJson(s)
    try {
      await navigator.clipboard.writeText(json)
    } catch {
      // Clipboard API unavailable (insecure context / denied) — fall back to a
      // hidden textarea so the copy still works instead of silently failing.
      const ta = document.createElement('textarea')
      ta.value = json
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      document.body.removeChild(ta)
    }
    toast.info(tr('server.copied'))
  }
  return (
    <div className="mx-auto max-w-2xl">
      {/* Stacks below `sm` (title, description, button) so a phone never
          squeezes the three into one row. */}
      <div className="mb-1 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <h2 className="flex min-w-0 flex-col gap-0.5 text-sm font-semibold sm:flex-row sm:items-center sm:gap-1">
          <span className="shrink-0">{tr('server.title')}</span>
          <InfoPopover text={tr('server.intro')} />
        </h2>
        <button
          data-testid="mcp-importable-open"
          onClick={openImportable}
          title={tr('server.otherServersHint')}
          className="shrink-0 self-start rounded-lg border sm:self-auto border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-1.5 text-xs font-medium hover:opacity-90"
        >
          {tr('server.otherServers')}
        </button>
      </div>
      <div className="mt-2 space-y-2">
        {servers.map((s) => {
          // This server's MCP tools — drives the per-server visibility quick action
          // (set the whole server's tools to one tier at once).
          const serverTools = tools.filter(
            (t) => toolSource(t) === 'mcp' && (toolServer(t) || 'MCP') === s.name,
          )
          return (
            <ServerRow
              key={s.id}
              server={s}
              toolCount={serverTools.length}
              poolStats={poolStats}
              testing={testing === s.id}
              testResult={testResult[s.id]}
              editing={editingId === s.id}
              onTest={() => onTest(s)}
              onCopy={() => copyServer(s)}
              onToggle={() => onToggle(s)}
              onEdit={() => onEdit(s)}
              onRemove={() => onRemove(s)}
              onVisibility={(tier) => onServerVisibility(s.name, tier)}
            />
          )
        })}
        {servers.length === 0 && (
          <p className="text-sm text-[var(--color-text-dim)]">{tr('server.empty')}</p>
        )}
      </div>

      {/* Add server */}
      <div className="mt-4 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
        <h3 className="mb-3 text-xs font-semibold text-[var(--color-text-dim)]">
          {tr('server.newTitle')}
        </h3>
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <input
            data-testid="mcp-server-name-input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={tr('server.namePlaceholder')}
            className="min-w-0 rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none"
          />
          <select
            data-testid="mcp-server-transport-select"
            value={transport}
            onChange={(e) => setTransport(e.target.value as MCPTransport)}
            className="min-w-0 rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none"
          >
            <option value="stdio">{tr('server.transportStdio')}</option>
            <option value="http">{tr('server.transportHttp')}</option>
          </select>
          {transport === 'stdio' ? (
            <>
              <input
                data-testid="mcp-server-command-input"
                value={command}
                onChange={(e) => setCommand(e.target.value)}
                placeholder={tr('server.commandPlaceholder')}
                className="min-w-0 rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none"
              />
              <input
                data-testid="mcp-server-args-input"
                value={argsText}
                onChange={(e) => setArgsText(e.target.value)}
                placeholder={tr('server.argsPlaceholder')}
                className="min-w-0 rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none"
              />
            </>
          ) : (
            <>
              <input
                data-testid="mcp-server-url-input"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder={tr('server.urlPlaceholder')}
                className="min-w-0 rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none sm:col-span-2"
              />
              <textarea
                data-testid="mcp-server-headers-input"
                value={headersText}
                onChange={(e) => setHeadersText(e.target.value)}
                spellCheck={false}
                placeholder={tr('server.headersPlaceholder')}
                className="h-20 resize-y rounded bg-[var(--color-surface-2)] px-3 py-2 font-mono text-xs outline-none sm:col-span-2"
              />
            </>
          )}
          <label className="flex flex-wrap items-center gap-2 text-xs text-[var(--color-text-dim)] sm:col-span-2">
            <span className="shrink-0">{tr('server.connectionScope')}</span>
            {scope === 'scoped' && poolStats && (
              <InfoPopover
                text={`${tr('server.scopedConnections')} ${
                  poolStats.idleSec > 0
                    ? tr('server.idleClose', { minutes: Math.round(poolStats.idleSec / 60) })
                    : tr('server.idleDisabled')
                }`}
              />
            )}
            <select
              data-testid="mcp-server-scope-select"
              value={scope}
              onChange={(e) => setScope(e.target.value as 'shared' | 'scoped')}
              className="min-w-0 max-w-full rounded bg-[var(--color-surface-2)] px-2 py-1.5 text-sm outline-none"
            >
              <option value="shared">{tr('server.scopeShared')}</option>
              <option value="scoped">{tr('server.scopeScoped')}</option>
            </select>
          </label>
        </div>
        <div className="mt-3 flex items-center gap-2">
          <button
            data-testid="mcp-server-add"
            onClick={onAdd}
            className="rounded-lg bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-[var(--color-on-accent)] hover:opacity-90"
          >
            {editingId ? tr('actions.save') : tr('actions.add')}
          </button>
          {editingId && (
            <button
              data-testid="mcp-server-cancel-edit"
              onClick={onCancelEdit}
              className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-4 py-2 text-sm font-medium hover:opacity-90"
            >
              {tr('actions.cancel')}
            </button>
          )}
        </div>
      </div>

      {/* Bulk import from a pasted mcpServers JSON document. */}
      <div className="mt-4 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
        <h3 className="mb-2 flex items-center gap-1 text-xs font-semibold text-[var(--color-text-dim)]">
          {tr('server.importJsonTitle')}
          <InfoPopover
            text={
              <>
                {tr('server.importJsonBefore')}{' '}
                <code className="text-[var(--color-text)]">mcpServers</code>{' '}
                {tr('server.importJsonAfter')}
              </>
            }
          />
        </h3>
        <textarea
          data-testid="mcp-import-textarea"
          value={importText}
          onChange={(e) => setImportText(e.target.value)}
          spellCheck={false}
          placeholder={
            '{\n  "mcpServers": {\n    "playwright": {\n      "command": "bunx",\n      "args": ["@playwright/mcp", "--browser", "chrome"]\n    }\n  }\n}'
          }
          className="h-40 w-full resize-y rounded bg-[var(--color-surface-2)] px-3 py-2 font-mono text-xs outline-none"
        />
        <div className="mt-2 flex items-center gap-3">
          <button
            data-testid="mcp-import-button"
            onClick={onImport}
            disabled={importing || !importText.trim()}
            className="rounded-lg bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-[var(--color-on-accent)] hover:opacity-90 disabled:opacity-50"
          >
            {importing ? tr('actions.importing') : tr('actions.import')}
          </button>
          {importMsg && (
            <span className="break-words text-xs text-[var(--color-text-dim)]">{importMsg}</span>
          )}
        </div>
      </div>

      {/* Popup: MCP servers from OTHER workspaces, one-click add into this one. */}
      {showImportable && (
        <ModalOverlay onClose={() => setShowImportable(false)}>
          <div
            data-testid="mcp-importable-overlay"
            className="flex max-h-[80vh] w-full max-w-lg flex-col rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-[var(--shadow-lg)]"
            onClick={(e) => e.stopPropagation()}
          >
            <PaneHeader
              title={tr('server.otherModalTitle')}
              info={tr('server.otherModalSubtitle')}
              right={
                <button
                  data-testid="mcp-importable-close"
                  onClick={() => setShowImportable(false)}
                  className="rounded p-1 text-lg leading-none text-[var(--color-text-dim)] hover:opacity-80"
                  aria-label={tr('actions.close')}
                >
                  {tr('actions.closeSymbol')}
                </button>
              }
            />
            <div className="flex-1 overflow-y-auto p-3">
              {importable.length === 0 ? (
                <EmptyState title={tr('server.noImportable')} />
              ) : (
                <div className="space-y-2">
                  {importable.map((item) => (
                    <div
                      key={`${item.workspaceId}:${item.server.id}`}
                      data-testid="mcp-importable-item"
                      className="flex items-center justify-between gap-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] p-3"
                    >
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="truncate font-medium">{item.server.name}</span>
                          <span className="rounded bg-[var(--color-surface)] px-1.5 py-0.5 text-xs text-[var(--color-text-dim)]">
                            {item.server.transport}
                          </span>
                          <span className="rounded bg-[var(--color-surface)] px-1.5 py-0.5 text-xs text-[var(--color-text-dim)]">
                            {item.workspaceName}
                          </span>
                        </div>
                        {item.server.description && (
                          <div className="mt-0.5 truncate text-xs text-[var(--color-text-dim)]">
                            {item.server.description}
                          </div>
                        )}
                        <div className="truncate text-xs text-[var(--color-text-dim)]">
                          {item.server.transport === 'stdio'
                            ? `${item.server.command} ${parseArgs(item.server.args).join(' ')}`.trim()
                            : item.server.url}
                        </div>
                      </div>
                      <button
                        data-testid="mcp-importable-add"
                        data-server-id={item.server.id}
                        onClick={() => onAddImportable(item)}
                        disabled={addingImportable === item.server.id}
                        className="flex-shrink-0 rounded-lg bg-[var(--color-accent)] px-3 py-1.5 text-xs font-medium text-[var(--color-on-accent)] hover:opacity-90 disabled:opacity-50"
                      >
                        {addingImportable === item.server.id
                          ? tr('actions.adding')
                          : tr('actions.add')}
                      </button>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        </ModalOverlay>
      )}
    </div>
  )
}
