import type { ArtifactKind } from '../../types'
import { Markdown } from '../markdown/Markdown'
import { CodeBlock } from '../markdown/CodeBlock'

interface Props {
  kind: ArtifactKind
  language?: string
  content: string
}

// ArtifactView renders an artifact's content according to its kind: markdown
// prose, syntax-highlighted code, sandboxed HTML, inline SVG, a Mermaid source
// block, or plain text. Used by the artifacts screen and the version preview.
export function ArtifactView({ kind, language, content }: Props) {
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
      // No Mermaid renderer bundled yet — show the source so it's still useful.
      return <CodeBlock code={content} lang="mermaid" />
    default:
      return (
        <pre className="whitespace-pre-wrap break-words rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] p-3 text-sm leading-relaxed">
          {content}
        </pre>
      )
  }
}
