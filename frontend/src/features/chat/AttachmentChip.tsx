import { X } from 'lucide-react'
import type { Attachment } from '@/types'
import { attachmentMeta, formatBytes, imageURL } from '@/shared/lib/attachments'

interface Props {
  attachment: Attachment
  // When set, a remove (x) button is shown (composer tray). Omitted in the
  // read-only message bubble.
  onRemove?: () => void
  // Local object URL for an image still being composed (before it has a server
  // relPath). Takes precedence over the persisted imageURL.
  previewURL?: string
  // While true, a subtle pulsing overlay marks the upload as in-flight.
  uploading?: boolean
  // When set, the chip is rendered as a button and calls this on click.
  onClick?: () => void
}

// AttachmentChip renders one attachment as either an image thumbnail or a compact
// file card (icon + name + size). Shared by the composer tray and the user bubble.
export function AttachmentChip({ attachment, onRemove, previewURL, uploading, onClick }: Props) {
  const { Icon, label, tint } = attachmentMeta(attachment.kind)
  const img = previewURL ?? imageURL(attachment)

  const Tag = onClick ? 'button' : 'div'
  const clickTitle =
    attachment.source === 'artifact'
      ? 'Artifactı aç'
      : attachment.kind === 'image'
        ? 'Görseli aç'
        : 'Dosyayı aç'
  const clickProps = onClick ? { type: 'button' as const, onClick, title: clickTitle } : {}

  return (
    <Tag {...clickProps} className={`group relative shrink-0${onClick ? ' cursor-pointer' : ''}`}>
      {img ? (
        <div
          className={`h-[52px] w-[52px] overflow-hidden rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] transition${onClick ? ' hover:opacity-80 hover:border-[var(--color-accent)]' : ''}`}
        >
          <img src={img} alt={attachment.name} className="h-full w-full object-cover" />
        </div>
      ) : (
        <div
          className={`flex h-[52px] w-[160px] items-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-2.5 transition${onClick ? ' hover:border-[var(--color-accent)] hover:bg-[var(--color-accent-soft)]' : ''}`}
        >
          <Icon size={20} className={`shrink-0 ${tint}`} />
          <span className="flex min-w-0 flex-col">
            <span className="truncate text-xs font-medium text-[var(--color-text)]">
              {attachment.source === 'artifact' ? '#' : ''}
              {attachment.name}
            </span>
            <span className="truncate text-[10px] text-[var(--color-text-dim)]">
              {attachment.source === 'artifact' ? 'Artifact' : label}
              {attachment.size > 0 ? ` · ${formatBytes(attachment.size)}` : ''}
            </span>
          </span>
        </div>
      )}

      {uploading && (
        <div className="absolute inset-0 flex items-center justify-center rounded-lg bg-black/40">
          <span className="h-4 w-4 animate-spin rounded-full border-2 border-white/40 border-t-white" />
        </div>
      )}

      {onRemove && (
        <button
          type="button"
          onClick={onRemove}
          title="Eki kaldır"
          className="absolute -right-1.5 -top-1.5 flex h-5 w-5 items-center justify-center rounded-full border border-[var(--color-border)] bg-[var(--color-surface-2)] text-[var(--color-text-dim)] opacity-0 transition group-hover:opacity-100 hover:text-[var(--color-danger)]"
        >
          <X size={12} />
        </button>
      )}
    </Tag>
  )
}
