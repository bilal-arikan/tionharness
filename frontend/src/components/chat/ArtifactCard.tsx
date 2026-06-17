import { FileCode } from 'lucide-react'
import type { TurnStep, ArtifactRefResult } from '../../types'

interface Props {
  step: TurnStep
  onOpenArtifact?: (id: string) => void
}

// parseArtifactResult decodes the JSON result emitted by the create_artifact /
// update_artifact tools ({id,title,kind,action}). Returns null when the tool
// failed (output is a plain error string).
function parseArtifactResult(output?: string): ArtifactRefResult | null {
  if (!output) return null
  try {
    const v = JSON.parse(output)
    if (v && typeof v.id === 'string') return v as ArtifactRefResult
  } catch {
    /* not JSON → tool error */
  }
  return null
}

const KIND_LABEL: Record<string, string> = {
  markdown: 'Markdown',
  code: 'Kod',
  html: 'HTML',
  text: 'Metin',
  svg: 'SVG',
  mermaid: 'Mermaid',
}

// ArtifactCard renders a create_artifact / update_artifact tool step as a
// clickable card that opens the artifact in the dedicated viewer — the chat
// equivalent of a Claude.ai artifact chip. On tool failure it falls back to a
// small error row.
export function ArtifactCard({ step, onOpenArtifact }: Props) {
  const ref = parseArtifactResult(step.output)

  if (!ref) {
    return (
      <div className="flex items-center gap-2 rounded-lg border border-red-500/30 bg-red-500/5 px-3 py-1.5 text-xs text-red-400">
        <FileCode size={14} className="shrink-0" />
        <span className="truncate">Artifact kaydedilemedi{step.output ? `: ${step.output}` : ''}</span>
      </div>
    )
  }

  return (
    <button
      onClick={() => onOpenArtifact?.(ref.id)}
      className="group flex w-full items-center gap-3 rounded-lg border border-[var(--color-accent-soft)] bg-[var(--color-accent-soft)] px-3 py-2 text-left transition hover:brightness-110"
    >
      <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-[var(--color-accent)] text-white">
        <FileCode size={16} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-medium text-[var(--color-accent)]">{ref.title}</span>
        <span className="block text-[11px] text-[var(--color-text-dim)]">
          {KIND_LABEL[ref.kind] ?? ref.kind} ·{' '}
          {ref.action === 'update' ? 'güncellendi' : 'oluşturuldu'}
        </span>
      </span>
      <span className="shrink-0 text-xs text-[var(--color-accent)] opacity-0 transition group-hover:opacity-100">
        Görüntüle →
      </span>
    </button>
  )
}
