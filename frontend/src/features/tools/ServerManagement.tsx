import { useState } from 'react'
import type {
  MCPServer,
  MCPTransport,
  MCPPoolStats,
  ToolVisibility,
  WorkspaceTool,
  ImportableMCPServer,
} from '@/types'
import { parseArgs, serverToImportJson, toolSource, toolServer, VISIBILITY_TIERS } from './toolMeta'
import { EmptyState, ModalOverlay, PaneHeader, toast } from '@/shared/components'

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
    toast.info('Panoya kopyalandı')
  }
  return (
    <div className="mx-auto max-w-2xl">
      <div className="mb-1 flex items-center justify-between gap-2">
        <h2 className="text-sm font-semibold">MCP Sunucuları</h2>
        <button
          data-testid="mcp-importable-open"
          onClick={openImportable}
          title="TionHarness'teki diğer workspace'lerde tanımlı, buraya eklenmemiş MCP sunucularını gör ve tek tıkla ekle."
          className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-1.5 text-xs font-medium hover:opacity-90"
        >
          Diğer MCP’ler
        </button>
      </div>
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
                    {s.scope === 'scoped' && (
                      <span
                        data-testid="mcp-server-scope-badge"
                        className="rounded bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-xs text-[var(--color-accent)]"
                        title="Session bazlı: her (oturum, ajan) için ayrı canlı bağlantı; boşta kalınca otomatik kapanır."
                      >
                        session bazlı
                      </span>
                    )}
                    {(() => {
                      // Live-connection / reaper indicator from the pool snapshot.
                      const st = poolStats?.servers.find((x) => x.server === s.name)
                      if (!st || st.live === 0) return null
                      const idleMin = poolStats ? Math.round(poolStats.idleSec / 60) : 0
                      return (
                        <span
                          data-testid="mcp-server-live-badge"
                          className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-xs text-[var(--color-text-secondary)]"
                          title={
                            st.scoped
                              ? `${st.live} canlı bağlantı (${st.total} slot). Boşta ${idleMin} dk sonra kapanır (reaper).`
                              : `${st.live} canlı paylaşımlı bağlantı (workspace geneli, reaper'a tabi değil).`
                          }
                        >
                          🔗 {st.live}
                          {st.scoped ? ' oturum' : ''}
                        </span>
                      )
                    })()}
                    {!s.enabled && (
                      <span className="text-xs text-[var(--color-text-dim)]">(devre dışı)</span>
                    )}
                  </div>
                  <div className="truncate text-xs text-[var(--color-text-dim)]">
                    {s.transport === 'stdio'
                      ? `${s.command} ${parseArgs(s.args).join(' ')}`.trim()
                      : s.url}
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
                    data-testid="mcp-server-copy"
                    data-server-id={s.id}
                    onClick={() => copyServer(s)}
                    title="Bu sunucunun yapılandırmasını mcpServers JSON'u olarak panoya kopyala (başka yere yapıştırıp içe aktarılabilir)."
                    className="rounded bg-[var(--color-surface-2)] px-2 py-1 text-xs hover:opacity-90"
                  >
                    Json
                  </button>
                  <button
                    data-testid="mcp-server-toggle"
                    data-server-id={s.id}
                    role="switch"
                    aria-checked={s.enabled}
                    onClick={() => onToggle(s)}
                    title={s.enabled ? 'Devre dışı bırak' : 'Etkinleştir'}
                    className={`relative h-5 w-9 flex-shrink-0 rounded-full transition ${
                      s.enabled ? 'bg-[var(--color-accent)]' : 'bg-[var(--color-surface-2)]'
                    }`}
                  >
                    <span
                      className={`absolute top-0.5 h-4 w-4 rounded-full bg-white transition-all ${
                        s.enabled ? 'left-4' : 'left-0.5'
                      }`}
                    />
                  </button>
                  <button
                    data-testid="mcp-server-edit"
                    data-server-id={s.id}
                    onClick={() => onEdit(s)}
                    title="Bu sunucuyu düzenle (isim, komut, URL, başlıklar, kapsam). Değişiklik sonraki turda yeniden bağlanır."
                    className={`rounded px-2 py-1 text-xs hover:opacity-90 ${
                      editingId === s.id
                        ? 'bg-[var(--color-accent)] text-[var(--color-on-accent)]'
                        : 'bg-[var(--color-surface-2)]'
                    }`}
                  >
                    Düzenle
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
                <div className="mt-2 break-words text-xs text-[var(--color-text-dim)]">
                  {testResult[s.id]}
                </div>
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
        <h3 className="mb-3 text-xs font-semibold text-[var(--color-text-dim)]">
          Yeni MCP sunucusu
        </h3>
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
                placeholder={
                  'Opsiyonel başlıklar — her satıra "Anahtar: Değer"\nör. Authorization: Bearer TOKEN'
                }
                className="col-span-2 h-20 resize-y rounded bg-[var(--color-surface-2)] px-3 py-2 font-mono text-xs outline-none"
              />
            </>
          )}
          <label className="col-span-2 flex items-center gap-2 text-xs text-[var(--color-text-secondary)]">
            <span className="shrink-0">Bağlantı kapsamı</span>
            <select
              data-testid="mcp-server-scope-select"
              value={scope}
              onChange={(e) => setScope(e.target.value as 'shared' | 'scoped')}
              className="rounded bg-[var(--color-surface-2)] px-2 py-1.5 text-sm outline-none"
            >
              <option value="shared">Paylaşımlı (workspace geneli, varsayılan)</option>
              <option value="scoped">Session bazlı (her oturuma ayrı, idle'da kapanır)</option>
            </select>
          </label>
        </div>
        {scope === 'scoped' && poolStats && (
          <p className="mt-2 text-xs text-[var(--color-text-dim)]">
            Session bazlı bağlantılar{' '}
            {poolStats.idleSec > 0
              ? `${Math.round(poolStats.idleSec / 60)} dk boşta kaldıktan sonra otomatik kapanır (reaper).`
              : 'idle eviction kapalı (reaper devre dışı).'}
          </p>
        )}
        <div className="mt-3 flex items-center gap-2">
          <button
            data-testid="mcp-server-add"
            onClick={onAdd}
            className="rounded-lg bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-[var(--color-on-accent)] hover:opacity-90"
          >
            {editingId ? 'Kaydet' : 'Ekle'}
          </button>
          {editingId && (
            <button
              data-testid="mcp-server-cancel-edit"
              onClick={onCancelEdit}
              className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-4 py-2 text-sm font-medium hover:opacity-90"
            >
              Vazgeç
            </button>
          )}
        </div>
      </div>

      {/* Bulk import from a pasted mcpServers JSON document. */}
      <div className="mt-4 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
        <h3 className="mb-1 text-xs font-semibold text-[var(--color-text-dim)]">
          JSON ile içe aktar
        </h3>
        <p className="mb-2 text-xs text-[var(--color-text-dim)]">
          Standart <code className="text-[var(--color-text)]">mcpServers</code> JSON'u yapıştır
          (Claude Code / .mcp.json biçimi). Birden çok sunucu tek seferde eklenir.
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
            className="rounded-lg bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-[var(--color-on-accent)] hover:opacity-90 disabled:opacity-50"
          >
            {importing ? 'İçe aktarılıyor…' : 'İçe aktar'}
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
              title="Diğer workspace’lerdeki MCP’ler"
              subtitle="Bu workspace’e eklenmemiş sunucular. Tek tıkla kopyala."
              right={
                <button
                  data-testid="mcp-importable-close"
                  onClick={() => setShowImportable(false)}
                  className="rounded p-1 text-lg leading-none text-[var(--color-text-dim)] hover:opacity-80"
                  aria-label="Kapat"
                >
                  ×
                </button>
              }
            />
            <div className="flex-1 overflow-y-auto p-3">
              {importable.length === 0 ? (
                <EmptyState title="Eklenebilecek başka MCP sunucusu yok." />
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
                        {addingImportable === item.server.id ? 'Ekleniyor…' : 'Ekle'}
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
