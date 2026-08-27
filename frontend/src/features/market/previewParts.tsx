import type { ReactNode } from 'react'
import { Globe2, type LucideIcon } from 'lucide-react'
import type { Pack, PackKind } from '@/types'
import type { PriceTable } from '@/api/providers'
import type { PreviewItem } from '@/api/ingest'
import { Markdown } from '@/shared/components/markdown/Markdown'
import { fmtPrice, KIND_LABEL, SOURCE_LABEL, stripFrontmatter } from './marketHelpers'
import { formatDate } from '@/shared/lib/intl'

// Row is a small labelled key/value line used in the agent/provider preview.
export function Row({ k, v }: { k: string; v?: string }) {
  if (!v) return null
  return (
    <div className="flex gap-2 text-xs">
      <span className="w-24 shrink-0 text-[var(--color-text-dim)]">{k}</span>
      <span className="min-w-0 break-words">{v}</span>
    </div>
  )
}

// CapBadge is a small capability pill (green when the capability is on, muted
// otherwise) used in the provider preview for cache / reasoning support.
export function CapBadge({ label, on }: { label: string; on: boolean }) {
  return (
    <span
      className="rounded px-2 py-0.5 text-[11px]"
      style={{
        background: on
          ? 'color-mix(in srgb, var(--color-success) 13%, transparent)'
          : 'var(--color-surface-2)',
        color: on ? 'var(--color-success)' : 'var(--color-text-dim)',
      }}
    >
      {label}
    </span>
  )
}

// ColumnsPreview renders a kanban column layout as colored chips (shared by the
// board and workspace previews).
export function ColumnsPreview({
  columns,
}: {
  columns?: { key: string; label: string; color?: string }[]
}) {
  if (!columns || columns.length === 0) return null
  return (
    <div className="flex flex-wrap gap-1.5">
      {columns.map((c) => (
        <span
          key={c.key}
          className="rounded px-2 py-0.5 text-[11px]"
          style={{
            background: c.color
              ? `color-mix(in srgb, ${c.color} 14%, transparent)`
              : 'var(--color-surface-2)',
            color: c.color || 'var(--color-text-dim)',
          }}
        >
          {c.label}
        </span>
      ))}
    </div>
  )
}

// ModelList renders a provider pack's models, each with its ballpark price
// (input / output per 1M tokens) when known. providerId is the pack slug used to
// look up prices[providerId][model].
export function ModelList({
  models,
  providerId,
  prices,
}: {
  models?: string
  providerId: string
  prices: PriceTable
}) {
  if (!models) return null
  const ids = models
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean)
  const table = prices[providerId] || {}
  return (
    <div className="pt-1">
      <span className="text-xs text-[var(--color-text-dim)]">Modeller ({ids.length})</span>
      <div className="mt-1 max-h-64 overflow-y-auto rounded bg-[var(--color-surface-2)] p-1.5">
        {ids.map((id) => {
          const pr = table[id]
          return (
            <div
              key={id}
              className="flex items-center justify-between gap-3 px-1 py-0.5 text-[11px]"
            >
              <span className="min-w-0 break-all font-mono">{id}</span>
              {pr ? (
                <span
                  className="shrink-0 tabular-nums text-[var(--color-text-dim)]"
                  title="giriş / çıkış — $/1M token"
                >
                  {fmtPrice(pr.inputPerMTok)} / {fmtPrice(pr.outputPerMTok)}
                </span>
              ) : (
                <span className="shrink-0 text-[var(--color-text-dim)] opacity-50">—</span>
              )}
            </div>
          )
        })}
      </div>
      <p className="mt-1 text-[10px] text-[var(--color-text-dim)]">
        $/1M token (giriş / çıkış) — yaklaşık liste fiyatı.
      </p>
    </div>
  )
}

// StatChip is one cell of the workspace stat strip. The icon is a monochrome
// lucide glyph (inherits the muted text color) rather than a colored emoji.
export function StatChip({
  icon: Icon,
  label,
  value,
}: {
  icon: LucideIcon
  label: string
  value: number
}) {
  return (
    <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-2 text-center">
      <div className="flex items-center justify-center gap-1 text-sm font-semibold">
        <Icon size={13} className="text-[var(--color-text-dim)]" /> {value}
      </div>
      <div className="text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
        {label}
      </div>
    </div>
  )
}

// PreviewSection is a titled block used inside the workspace preview.
export function PreviewSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div>
      <div className="mb-1 text-[11px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        {title}
      </div>
      {children}
    </div>
  )
}

// MiniChip is a small inline label (modes, skills, flow tags).
// title is optional and carries the hover explanation for chips whose label is a
// term rather than a value (e.g. "koordinatör", "tek sahip") — a pack preview is
// read by someone deciding whether to install, so an unexplained term is a dead end.
export function MiniChip({ children, title }: { children: ReactNode; title?: string }) {
  return (
    <span
      title={title}
      className="rounded bg-[var(--color-bg)] px-1.5 py-0.5 text-[10px] text-[var(--color-text-dim)]"
    >
      {children}
    </span>
  )
}

// PackMeta renders a row of metadata chips (source, installed version, created
// date, tags) shown under the description for every pack kind.
export function PackMeta({ pack }: { pack: Pack }) {
  const created = pack.createdAt
    ? formatDate(new Date(pack.createdAt * 1000), { dateStyle: 'short' })
    : null
  const src = pack.source ? (SOURCE_LABEL[pack.source] ?? pack.source) : null
  if (!src && !created && !pack.installedVersion && !(pack.tags && pack.tags.length)) return null
  return (
    <div className="mt-2 flex flex-wrap items-center gap-1.5">
      {src && (
        <MiniChip>
          {src}
          {pack.registryName ? ` · ${pack.registryName}` : ''}
        </MiniChip>
      )}
      {pack.installedVersion && <MiniChip>Kurulu: v{pack.installedVersion}</MiniChip>}
      {created && <MiniChip>📅 {created}</MiniChip>}
      {pack.tags?.map((t) => (
        <MiniChip key={t}>#{t}</MiniChip>
      ))}
    </div>
  )
}

// SourceRefPreview renders the GitHub-fetched preview of a directory-site (source-ref)
// catalog entry: the source link plus each discovered artifact's rendered body.
export function SourceRefPreview({
  url,
  items,
  loading,
}: {
  url: string
  items: PreviewItem[] | null
  loading: boolean
}) {
  return (
    <div className="space-y-3">
      <a
        href={url}
        target="_blank"
        rel="noreferrer"
        className="flex items-center gap-1.5 text-xs text-[var(--color-accent)] hover:underline"
      >
        <Globe2 size={12} /> {url}
      </a>
      {loading && (
        <p className="text-xs text-[var(--color-text-dim)]">Önizleme GitHub'dan yükleniyor…</p>
      )}
      {!loading && items && items.length === 0 && (
        <p className="text-xs text-[var(--color-text-dim)]">
          Önizleme alınamadı (kurulumda yine de denenir).
        </p>
      )}
      {!loading &&
        items &&
        items.map((it, i) => (
          <div key={i} className="space-y-1">
            {items.length > 1 && (
              <div className="flex items-center gap-1.5 text-xs font-medium">
                <KindBadge kind={it.kind} /> <code>{it.slug}</code>
              </div>
            )}
            {it.warnings && it.warnings.length > 0 && (
              <p className="text-[10px] text-[var(--color-warning,#d97706)]">
                {it.warnings.join(' · ')}
              </p>
            )}
            <div className="text-xs">
              <Markdown>{stripFrontmatter(it.body || it.description || '')}</Markdown>
            </div>
          </div>
        ))}
    </div>
  )
}

export function KindBadge({ kind }: { kind: PackKind }) {
  return (
    <span className="rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide bg-[var(--color-surface-2)] text-[var(--color-text-dim)]">
      {KIND_LABEL[kind]}
    </span>
  )
}
