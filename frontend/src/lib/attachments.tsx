// Attachment presentation helpers: map an attachment kind to an icon + label and
// format byte sizes. Mirrors the coarse kinds set by the backend (attachmentKind).
import type { ComponentType } from 'react'
import {
  FileText,
  FileCode,
  FileImage,
  FileArchive,
  FileAudio,
  FileVideo,
  File as FileIcon,
} from 'lucide-react'
import type { Attachment, AttachmentKind } from '../types'
import { getActiveWorkspace } from '../api/client'

interface IconMeta {
  Icon: ComponentType<{ size?: number; className?: string }>
  label: string
  // Tailwind text-color class for the icon tint.
  tint: string
}

const KIND_META: Record<AttachmentKind, IconMeta> = {
  image: { Icon: FileImage, label: 'Görsel', tint: 'text-[var(--color-accent)]' },
  text: { Icon: FileText, label: 'Metin', tint: 'text-[var(--color-text-dim)]' },
  code: { Icon: FileCode, label: 'Kod', tint: 'text-[var(--color-success)]' },
  pdf: { Icon: FileText, label: 'PDF', tint: 'text-[var(--color-danger)]' },
  office: { Icon: FileText, label: 'Belge', tint: 'text-[var(--color-accent)]' },
  archive: { Icon: FileArchive, label: 'Arşiv', tint: 'text-[var(--color-warning)]' },
  audio: { Icon: FileAudio, label: 'Ses', tint: 'text-[var(--color-accent)]' },
  video: { Icon: FileVideo, label: 'Video', tint: 'text-[var(--color-accent)]' },
  file: { Icon: FileIcon, label: 'Dosya', tint: 'text-[var(--color-text-dim)]' },
}

export function attachmentMeta(kind: AttachmentKind): IconMeta {
  return KIND_META[kind] ?? KIND_META.file
}

// formatBytes renders a compact human size (e.g. "12 KB", "1.4 MB").
export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${Math.round(n / 1024)} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}

// imageURL builds the inline-serving URL for a persisted image attachment. The
// /api/files endpoint resolves the workspace-relative `rel` path against the
// active workspace sandbox; the workspace id rides as a query param because an
// <img> tag cannot send the X-Workspace-Id header. Returns null for non-image
// kinds or attachments not written to disk (pure pasted text).
export function imageURL(a: Attachment): string | null {
  if (a.kind !== 'image' || !a.relPath) return null
  const ws = getActiveWorkspace()
  const wsq = ws ? `&ws=${encodeURIComponent(ws)}` : ''
  return `/api/files?rel=${encodeURIComponent(a.relPath)}${wsq}`
}

// PASTE_AS_FILE_THRESHOLD: pasted text longer than this becomes a text attachment
// instead of going into the textarea (Claude.ai-style "pasted text").
export const PASTE_AS_FILE_THRESHOLD = 2000
