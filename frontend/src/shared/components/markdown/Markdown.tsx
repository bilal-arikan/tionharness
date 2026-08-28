import { memo, useMemo, useState, type ReactNode } from 'react'
import ReactMarkdown, { type Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { CodeBlock } from './CodeBlock'
import { Lightbox } from '@/shared/components'
import { isExternalUrl, isImagePath, isVideoPath, mediaUrl } from '@/shared/lib/paths'

interface Props {
  children: string
  onOpenFile?: (path: string) => void
}

// Flatten react-markdown's code children (string | string[]) to a single string.
function flatten(node: ReactNode): string {
  if (typeof node === 'string') return node
  if (Array.isArray(node)) return node.map(flatten).join('')
  return ''
}

// Markdown treats backslashes as escapes, which mangles Windows paths inside
// link/image targets (![x](C:\a\b.png)). Normalise backslashes to forward
// slashes within ](...) targets so local paths survive parsing.
function normalizeWindowsPaths(src: string): string {
  return src.replace(
    /(!?\]\()([^)]+)(\))/g,
    (_m, a, target, c) => a + target.replace(/\\/g, '/') + c,
  )
}

// A whole line that is nothing but a single markdown image: ![alt](url "title").
const IMAGE_LINE_RE = /^!\[([^\]]*)\]\(\s*<?([^)\s]+)>?(?:\s+"[^"]*")?\s*\)$/

// groupMediaRuns rewrites runs of 2+ consecutive image-only lines (optionally
// separated by blank lines) into a single ```gallery fenced block, so a message
// that lists several images renders as one gallery instead of a tall stack. A
// lone image, or an image embedded in a line of prose, is left untouched and
// renders inline. Runs are emitted as JSON so the Gallery parser handles them.
function groupMediaRuns(text: string): string {
  const lines = text.split('\n')
  const out: string[] = []
  let run: { alt: string; src: string }[] = []
  let inFence = false

  const flush = () => {
    if (run.length >= 2) {
      out.push('```gallery')
      out.push(
        JSON.stringify({ images: run.map((r) => ({ src: r.src, alt: r.alt || undefined })) }),
      )
      out.push('```')
    } else if (run.length === 1) {
      out.push(`![${run[0].alt}](${run[0].src})`)
    }
    run = []
  }

  for (const line of lines) {
    const t = line.trim()
    // Never touch content inside fenced code blocks (``` or ~~~).
    if (/^(```|~~~)/.test(t)) {
      flush()
      inFence = !inFence
      out.push(line)
      continue
    }
    if (inFence) {
      out.push(line)
      continue
    }
    if (t === '') {
      // Blank line: hold the run open (consecutive images may span paragraphs);
      // drop the blank when inside a run, otherwise keep it.
      if (run.length === 0) out.push(line)
      continue
    }
    const m = IMAGE_LINE_RE.exec(t)
    if (m) {
      run.push({ alt: m[1], src: m[2] })
      continue
    }
    flush()
    out.push(line)
  }
  flush()
  return out.join('\n')
}

// remarkGfm is the only plugin and holds no per-render state, so the array is
// hoisted: a fresh [remarkGfm] each render would be a new prop identity and
// defeat MarkdownBody's memo.
const REMARK_PLUGINS = [remarkGfm]

// MarkdownBody is the actual parse+render, split out and memoised so it re-runs
// ONLY when the source text or the component map changes. Without the split,
// local state in the wrapper (the lightbox `zoom`) would re-parse the entire
// document on every open/close — an image click re-parsing a long reply.
const MarkdownBody = memo(function MarkdownBody({
  source,
  components,
}: {
  source: string
  components: Components
}) {
  return (
    <ReactMarkdown
      remarkPlugins={REMARK_PLUGINS}
      // Disable the default URL sanitiser: it treats "C:" as an unknown
      // protocol and strips local Windows paths. We resolve links/images
      // ourselves in the components below.
      urlTransform={(u) => u}
      components={components}
    >
      {source}
    </ReactMarkdown>
  )
})

// Markdown renders assistant content as GitHub-flavored markdown: headings,
// lists, tables, task lists, fenced code with syntax highlight + diffs, inline
// images (local paths resolved through /api/files) and clickable links.
function MarkdownImpl({ children, onOpenFile }: Props) {
  const [zoom, setZoom] = useState<{ src: string; alt: string } | null>(null)
  // Both preprocessors do a full scan of the source (a regex sweep and a
  // line-by-line pass), so they are keyed on the text rather than re-run on
  // every render — during streaming the text changes, but a lightbox toggle or
  // a parent re-render then costs nothing.
  const source = useMemo(() => groupMediaRuns(normalizeWindowsPaths(children)), [children])
  const components = useMemo<Components>(
    () => ({
      // CodeBlock supplies its own <pre>; passthrough avoids a nested pre.
      pre({ children }) {
        return <>{children}</>
      },
      code({ className, children }) {
        const match = /language-([\w-]+)/.exec(className || '')
        const text = flatten(children as ReactNode)
        // Block code: has a language fence, or is multi-line.
        if (match || text.includes('\n')) {
          return <CodeBlock code={text.replace(/\n$/, '')} lang={match?.[1]} />
        }
        return (
          <code className="rounded bg-[var(--color-surface-2)] px-1 py-0.5 text-[0.85em] text-[var(--color-accent)]">
            {children}
          </code>
        )
      },
      a({ href, children }) {
        const url = href || ''
        // Local file path (not http, not anchor) → open-file callback.
        if (url && !isExternalUrl(url) && !url.startsWith('#') && !url.startsWith('mailto:')) {
          return (
            <button
              type="button"
              onClick={() => onOpenFile?.(url)}
              className="break-all font-mono text-[0.92em] text-[var(--color-accent)] underline decoration-dotted underline-offset-2 hover:opacity-80"
            >
              {children}
            </button>
          )
        }
        return (
          <a
            href={url}
            target="_blank"
            rel="noreferrer"
            className="text-[var(--color-accent)] underline underline-offset-2 hover:opacity-80"
          >
            {children}
          </a>
        )
      },
      img({ src, alt }) {
        const raw = typeof src === 'string' ? src : ''
        // Resolve local media paths through the backend file server.
        const resolved =
          isExternalUrl(raw) || raw.startsWith('data:')
            ? raw
            : isImagePath(raw) || isVideoPath(raw)
              ? mediaUrl(raw)
              : raw
        // A markdown image whose target is a video (![alt](clip.mp4)) renders
        // as an inline <video> player instead of a broken <img>.
        if (isVideoPath(raw)) {
          return (
            <video
              src={resolved}
              controls
              className="my-2 max-h-96 rounded-lg border border-[var(--color-border)]"
            />
          )
        }
        return (
          <img
            src={resolved}
            alt={alt || ''}
            onClick={() => setZoom({ src: resolved, alt: alt || '' })}
            className="my-2 max-h-96 cursor-zoom-in rounded-lg border border-[var(--color-border)] transition hover:opacity-90"
          />
        )
      },
    }),
    [onOpenFile],
  )
  return (
    <div className="sg-markdown text-sm leading-relaxed">
      <MarkdownBody source={source} components={components} />
      {zoom && (
        <Lightbox
          imageSrc={zoom.src}
          imageAlt={zoom.alt}
          title={zoom.alt}
          onClose={() => setZoom(null)}
        />
      )}
    </div>
  )
}

export const Markdown = memo(MarkdownImpl)
