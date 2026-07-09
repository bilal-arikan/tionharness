import type { MCPServer, MCPTransport, ToolVisibility, WorkspaceTool } from '@/types'
import { parseArgs, toolSource, toolServer, VISIBILITY_TIERS } from './toolMeta'

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
  onAdd: () => void
  onToggle: (s: MCPServer) => void
  onTest: (s: MCPServer) => void
  onRemove: (s: MCPServer) => void
  importText: string
  setImportText: (v: string) => void
  importing: boolean
  importMsg: string
  onImport: () => void
}) {
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
    onAdd,
    onToggle,
    onTest,
    onRemove,
    importText,
    setImportText,
    importing,
    importMsg,
    onImport,
  } = props
  return (
    <div className="mx-auto max-w-2xl">
      <h2 className="mb-1 text-sm font-semibold">MCP Sunucuları</h2>
      <p className="mb-3 text-xs text-[var(--color-text-dim)]">
        Soldaki listeden bir araç seçerek detaylarını görüntüleyip aç/kapatabilirsin.
      </p>
      <div className="space-y-2">
        {servers.map((s) => {
          // This server's MCP tools — drives the per-server visibility quick action
          // (set the whole server's tools to one tier at once).
          const serverTools = tools.filter(
            (t) => toolSource(t) === 'mcp' && (toolServer(t) || 'MCP') === s.name,
          )
          return (
          <div
            key={s.id}
            className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
          >
            <div className="flex items-center justify-between gap-2">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <span className="font-medium">{s.name}</span>
                  <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-xs text-[var(--color-text-dim)]">
                    {s.transport}
                  </span>
                  {s.command.toLowerCase().includes('codebase-memory-mcp') && (
                    <span
                      data-testid="mcp-server-isolated-store"
                      className="rounded bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-xs text-[var(--color-accent)]"
                      title="Bu sunucu workspace'e özel izole bir indeks store kullanır (CBM_CACHE_DIR = <workspace>/cbm-store) — indeksler workspace'ler arası karışmaz."
                    >
                      izole store
                    </span>
                  )}
                  {!s.enabled && <span className="text-xs text-[var(--color-text-dim)]">(devre dışı)</span>}
                </div>
                <div className="truncate text-xs text-[var(--color-text-dim)]">
                  {s.transport === 'stdio' ? `${s.command} ${parseArgs(s.args).join(' ')}`.trim() : s.url}
                </div>
              </div>
              <div className="flex flex-shrink-0 items-center gap-2">
                <button
                  data-testid="mcp-server-test"
                  data-server-id={s.id}
                  onClick={() => onTest(s)}
                  disabled={testing === s.id}
                  className="rounded bg-[var(--color-surface-2)] px-2 py-1 text-xs hover:opacity-90"
                >
                  Test
                </button>
                <button
                  data-testid="mcp-server-toggle"
                  data-server-id={s.id}
                  onClick={() => onToggle(s)}
                  className="rounded bg-[var(--color-surface-2)] px-2 py-1 text-xs hover:opacity-90"
                >
                  {s.enabled ? 'Kapat' : 'Aç'}
                </button>
                <button
                  data-testid="mcp-server-delete"
                  data-server-id={s.id}
                  onClick={() => onRemove(s)}
                  className="rounded px-2 py-1 text-xs text-[var(--color-danger)] hover:bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)]"
                >
                  Sil
                </button>
              </div>
            </div>
            {serverTools.length > 0 && (
              <div className="mt-2 flex flex-wrap items-center gap-2 border-t border-[var(--color-border)] pt-2">
                <span className="text-xs text-[var(--color-text-dim)]">
                  Tüm araçlar ({serverTools.length}) →
                </span>
                {VISIBILITY_TIERS.map((tier) => (
                  <button
                    key={tier.value}
                    data-testid="mcp-server-visibility-all"
                    data-server-id={s.id}
                    data-tier={tier.value}
                    onClick={() => onServerVisibility(s.name, tier.value)}
                    title={tier.hint}
                    className="rounded bg-[var(--color-surface-2)] px-2 py-1 text-xs hover:opacity-90"
                  >
                    {tier.label}
                  </button>
                ))}
              </div>
            )}
            {testResult[s.id] && (
              <div className="mt-2 break-words text-xs text-[var(--color-text-dim)]">{testResult[s.id]}</div>
            )}
          </div>
          )
        })}
        {servers.length === 0 && (
          <p className="text-sm text-[var(--color-text-dim)]">Henüz MCP sunucusu eklenmedi.</p>
        )}
      </div>

      {/* Add server */}
      <div className="mt-4 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
        <h3 className="mb-3 text-xs font-semibold text-[var(--color-text-dim)]">Yeni MCP sunucusu</h3>
        <div className="grid grid-cols-2 gap-2">
          <input
            data-testid="mcp-server-name-input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="İsim (ör. filesystem)"
            className="rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none"
          />
          <select
            data-testid="mcp-server-transport-select"
            value={transport}
            onChange={(e) => setTransport(e.target.value as MCPTransport)}
            className="rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none"
          >
            <option value="stdio">stdio</option>
            <option value="http">http (Streamable HTTP)</option>
          </select>
          {transport === 'stdio' ? (
            <>
              <input
                data-testid="mcp-server-command-input"
                value={command}
                onChange={(e) => setCommand(e.target.value)}
                placeholder="Komut (ör. npx)"
                className="rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none"
              />
              <input
                data-testid="mcp-server-args-input"
                value={argsText}
                onChange={(e) => setArgsText(e.target.value)}
                placeholder="Argümanlar (boşlukla ayrılmış)"
                className="rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none"
              />
            </>
          ) : (
            <>
              <input
                data-testid="mcp-server-url-input"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder="URL (Streamable HTTP endpoint)"
                className="col-span-2 rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none"
              />
              <textarea
                data-testid="mcp-server-headers-input"
                value={headersText}
                onChange={(e) => setHeadersText(e.target.value)}
                spellCheck={false}
                placeholder={'Opsiyonel başlıklar — her satıra "Anahtar: Değer"\nör. Authorization: Bearer TOKEN'}
                className="col-span-2 h-20 resize-y rounded bg-[var(--color-surface-2)] px-3 py-2 font-mono text-xs outline-none"
              />
            </>
          )}
        </div>
        <button
          data-testid="mcp-server-add"
          onClick={onAdd}
          className="mt-3 rounded-lg bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-white hover:opacity-90"
        >
          Ekle
        </button>
      </div>

      {/* Bulk import from a pasted mcpServers JSON document. */}
      <div className="mt-4 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
        <h3 className="mb-1 text-xs font-semibold text-[var(--color-text-dim)]">JSON ile içe aktar</h3>
        <p className="mb-2 text-xs text-[var(--color-text-dim)]">
          Standart <code className="text-[var(--color-text)]">mcpServers</code> JSON'u yapıştır (Claude Code /
          .mcp.json biçimi). Birden çok sunucu tek seferde eklenir.
        </p>
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
            className="rounded-lg bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
          >
            {importing ? 'İçe aktarılıyor…' : 'İçe aktar'}
          </button>
          {importMsg && <span className="break-words text-xs text-[var(--color-text-dim)]">{importMsg}</span>}
        </div>
      </div>
    </div>
  )
}
