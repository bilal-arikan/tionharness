import type { ToolVisibility, WorkspaceTool } from '@/types'
import { toolSource, toolServer, toolLabel, visibilityMeta, type ParamRow } from './toolMeta'
import { toolIcon } from '@/shared/lib/toolIcons'
import { VisibilityBadge, VisibilitySelector } from './VisibilityControls'

// ToolDetail renders the right-hand detail view for one selected tool.
export function ToolDetail({
  tool,
  params,
  saving,
  visBusy,
  onToggle,
  onSetVisibility,
}: {
  tool: WorkspaceTool
  params: ParamRow[]
  saving: boolean
  visBusy: boolean
  onToggle: () => void
  onSetVisibility: (tier: ToolVisibility) => void
}) {
  const ToolIcon = toolIcon(tool.name)
  const examples = (tool.examples ?? []) as unknown[]
  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <ToolIcon size={18} className="flex-shrink-0 text-[var(--color-accent)]" />
            <h2 className="truncate text-lg font-semibold">{toolLabel(tool)}</h2>
          </div>
          <div className="mt-1.5 flex flex-wrap items-center gap-2">
            <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-xs text-[var(--color-text-dim)]">
              {toolSource(tool) === 'mcp' ? `MCP · ${toolServer(tool)}` : 'Yerleşik'}
            </span>
            <span
              className={`rounded px-1.5 py-0.5 text-xs ${
                tool.enabled
                  ? 'bg-[color-mix(in_srgb,var(--color-success)_12%,transparent)] text-[var(--color-success)]'
                  : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
              }`}
            >
              {tool.enabled ? 'Aktif' : 'Devre dışı'}
            </span>
            <VisibilityBadge visibility={tool.visibility} />
          </div>
        </div>
        <div className="flex flex-shrink-0 items-center gap-2">
          <button
            data-testid="tool-detail-toggle"
            onClick={onToggle}
            disabled={saving}
            className={`rounded-lg px-4 py-2 text-sm font-medium transition disabled:opacity-50 ${
              tool.enabled
                ? 'bg-[var(--color-surface-2)] hover:opacity-90'
                : 'bg-[var(--color-accent)] text-white hover:opacity-90'
            }`}
          >
            {tool.enabled ? 'Devre dışı bırak' : 'Etkinleştir'}
          </button>
        </div>
      </div>

      {toolSource(tool) === 'mcp' && (
        <div>
          <code className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-xs">{tool.name}</code>
        </div>
      )}

      <section>
        <h3 className="mb-1.5 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
          Bağlam görünürlüğü
        </h3>
        <VisibilitySelector value={tool.visibility} busy={visBusy} onSelect={onSetVisibility} />
        <p className="mt-2 text-xs text-[var(--color-text-dim)]">{visibilityMeta(tool.visibility).hint}</p>
      </section>

      <section>
        <h3 className="mb-1.5 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
          Açıklama
        </h3>
        <p className="whitespace-pre-wrap text-sm leading-relaxed">
          {tool.description || <span className="text-[var(--color-text-dim)]">Açıklama yok.</span>}
        </p>
      </section>

      <section>
        <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
          Parametreler ({params.length})
        </h3>
        {params.length === 0 ? (
          <p className="text-sm text-[var(--color-text-dim)]">Bu araç parametre almıyor.</p>
        ) : (
          <div className="divide-y divide-[var(--color-border)] overflow-hidden rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)]">
            {params.map((p) => (
              <div key={p.name} className="px-4 py-2.5">
                <div className="flex items-center gap-2">
                  <code className="text-sm font-medium">{p.name}</code>
                  <span className="text-xs text-[var(--color-text-dim)]">{p.type}</span>
                  {p.required && (
                    <span className="rounded bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)] px-1.5 py-0.5 text-[10px] font-medium text-[var(--color-danger)]">
                      zorunlu
                    </span>
                  )}
                </div>
                {p.description && (
                  <p className="mt-1 text-xs text-[var(--color-text-dim)]">{p.description}</p>
                )}
              </div>
            ))}
          </div>
        )}
      </section>

      {examples.length > 0 && (
        <section>
          <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
            Örnek çağrılar ({examples.length})
          </h3>
          <p className="mb-2 text-xs text-[var(--color-text-dim)]">
            Şemanın ifade edemediği kullanım kalıpları (tarih/ID biçimi, birlikte gelen alanlar).
            Bunlar yalnızca <b>Tam</b> görünürlükte (tam şemayla) modele gider.
          </p>
          <div className="space-y-2">
            {examples.map((ex, i) => (
              <pre
                key={i}
                className="overflow-x-auto rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs"
              >
                {JSON.stringify(ex, null, 2)}
              </pre>
            ))}
          </div>
        </section>
      )}
    </div>
  )
}
