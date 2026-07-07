// Presentational metadata + small helpers for the artifacts screen, split out of
// ArtifactsPanel to keep that file focused on state/behaviour. Pure data and
// stateless helpers only.
import {
  FileText, Code2, Globe, Image, GitBranch,
  FileVideo, FileAudio, File as FileIcon,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { ArtifactKind } from '@/types'

export const KIND_ICON: Record<ArtifactKind, LucideIcon> = {
  markdown: FileText,
  code: Code2,
  html: Globe,
  text: FileText,
  svg: Image,
  mermaid: GitBranch,
  image: Image,
  video: FileVideo,
  audio: FileAudio,
  file: FileIcon,
}

export const KIND_LABEL: Record<ArtifactKind, string> = {
  markdown: 'Markdown',
  code: 'Kod',
  html: 'HTML',
  text: 'Metin',
  svg: 'SVG',
  mermaid: 'Mermaid',
  image: 'Görsel',
  video: 'Video',
  audio: 'Ses',
  file: 'Dosya',
}

// Origin badge: where the artifact came from. Tinted to read at a glance.
const ORIGIN_META: Record<string, { label: string; cls: string }> = {
  chat: { label: 'Sohbet eki', cls: 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]' },
  manual: { label: 'Manuel', cls: 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]' },
  agent: { label: 'Ajan', cls: 'bg-[color-mix(in_srgb,var(--color-success)_18%,transparent)] text-[var(--color-success)]' },
  tool: { label: 'Tool', cls: 'bg-[color-mix(in_srgb,var(--color-warning)_18%,transparent)] text-[var(--color-warning)]' },
  plan: { label: '📋 Plan', cls: 'bg-[color-mix(in_srgb,var(--color-accent)_22%,transparent)] text-[var(--color-accent)]' },
}

export function OriginBadge({ origin }: { origin?: string }) {
  const meta = origin ? ORIGIN_META[origin] : undefined
  if (!meta) return null
  return (
    <span className={`rounded px-1.5 py-0.5 text-[10px] font-medium ${meta.cls}`}>{meta.label}</span>
  )
}

// Manually creatable kinds (text-based). Media/file kinds arrive via drag-drop.
export const KINDS: ArtifactKind[] = ['markdown', 'code', 'html', 'text', 'svg', 'mermaid']

// Media/file kinds whose body lives on disk (sourcePath), not in an editable
// text field — these are added by dropping files, not typed.
const MEDIA_KINDS = new Set<ArtifactKind>(['image', 'video', 'audio', 'file'])
export const isMediaKind = (k: ArtifactKind) => MEDIA_KINDS.has(k)

// artifactKindForUpload maps an uploaded attachment (coarse backend kind + name)
// to the artifact kind used to render it. Media stays media; small text/code/
// markdown gets its native renderer; everything else is a stored file card.
export function artifactKindForUpload(att: { kind: string; name: string; textContent?: string }): ArtifactKind {
  switch (att.kind) {
    case 'image':
      return 'image'
    case 'video':
      return 'video'
    case 'audio':
      return 'audio'
  }
  const lower = att.name.toLowerCase()
  if (att.textContent) {
    if (lower.endsWith('.md') || lower.endsWith('.markdown')) return 'markdown'
    if (att.kind === 'code') return 'code'
    if (att.kind === 'text') return 'text'
  }
  return 'file'
}
