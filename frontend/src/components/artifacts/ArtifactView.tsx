import { useState } from 'react'
import { File as FileIcon } from 'lucide-react'
import type { ArtifactKind } from '../../types'
import { Markdown } from '../markdown/Markdown'
import { CodeBlock } from '../markdown/CodeBlock'
import { Lightbox } from '../common'
import { fileURL } from '../../lib/attachments'

// ImageArtifact renders an image artifact with click-to-zoom into the shared
// Lightbox (zoom + pan). Kept as its own component so the hook is valid even
// though ArtifactView itself is a switch with early returns.
function ImageArtifact({ url, alt }: { url: string; alt: string }) {
  const [zoom, setZoom] = useState(false)
  return (
    <div className="flex justify-center rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] p-4">
      <img
        src={url}
        alt={alt}
        onClick={() => setZoom(true)}
        className="max-h-[70vh] max-w-full cursor-zoom-in rounded transition hover:opacity-90"
      />
      {zoom && <Lightbox imageSrc={url} imageAlt={alt} title={alt} onClose={() => setZoom(false)} />}
    </div>
  )
}

interface Props {
  kind: ArtifactKind
  language?: string
  content: string
  // Workspace-relative path for media/file kinds (image/video/audio/file).
  sourcePath?: string
}

// ArtifactView renders an artifact's content according to its kind: markdown
// prose, syntax-highlighted code, sandboxed HTML, inline SVG, a Mermaid source
// block, plain text, or a media file (image/video/audio) served from sourcePath.
// Used by the artifacts screen and the version preview.
export function ArtifactView({ kind, language, content, sourcePath }: Props) {
  switch (kind) {
    case 'markdown':
      return <Markdown>{content}</Markdown>
    case 'code':
      return <CodeBlock code={content} lang={language || undefined} />
    case 'html':
      return (
        <iframe
          // Sandboxed: scripts run but cannot reach the parent (no allow-same-origin),
          // so artifact HTML can be interactive yet isolated from the app.
          sandbox="allow-scripts"
          srcDoc={content}
          title="HTML artifact"
          className="h-full min-h-[60vh] w-full rounded-lg border border-[var(--color-border)] bg-white"
        />
      )
    case 'svg':
      return (
        <div
          className="flex justify-center rounded-lg border border-[var(--color-border)] bg-white p-4"
          // SVG is agent-authored content shown in an isolated preview area.
          dangerouslySetInnerHTML={{ __html: content }}
        />
      )
    case 'mermaid':
      // Renders as a diagram via CodeBlock → MermaidDiagram (with Source/Expand/Copy).
      return <CodeBlock code={content} lang="mermaid" />
    case 'image': {
      const url = fileURL(sourcePath)
      return url ? <ImageArtifact url={url} alt={content || 'image artifact'} /> : <MissingMedia />
    }
    case 'video': {
      const url = fileURL(sourcePath)
      return url ? (
        <div className="flex justify-center rounded-lg border border-[var(--color-border)] bg-black p-2">
          <video src={url} controls className="max-h-[70vh] max-w-full rounded" />
        </div>
      ) : (
        <MissingMedia />
      )
    }
    case 'audio': {
      const url = fileURL(sourcePath)
      return url ? (
        <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] p-4">
          <audio src={url} controls className="w-full" />
        </div>
      ) : (
        <MissingMedia />
      )
    }
    case 'file': {
      const url = fileURL(sourcePath)
      return (
        <div className="flex items-center gap-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] p-4">
          <FileIcon size={28} className="shrink-0 text-[var(--color-text-dim)]" />
          <div className="min-w-0">
            <div className="truncate text-sm font-medium">{content || sourcePath || 'Dosya'}</div>
            <div className="text-xs text-[var(--color-text-dim)]">
              {url ? (
                <a href={url} target="_blank" rel="noreferrer" className="text-[var(--color-accent)] hover:underline">
                  Aç / indir
                </a>
              ) : (
                'Çalışma alanında saklı dosya'
              )}
            </div>
          </div>
        </div>
      )
    }
    default:
      return (
        <pre className="whitespace-pre-wrap break-words rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] p-3 text-sm leading-relaxed">
          {content}
        </pre>
      )
  }
}

// MissingMedia is shown when a media artifact has no resolvable source path.
function MissingMedia() {
  return (
    <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] p-4 text-sm text-[var(--color-text-dim)]">
      Medya dosyası bulunamadı (kaynak yol eksik).
    </div>
  )
}
