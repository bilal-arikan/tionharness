import { memo, type ReactNode } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { CodeBlock } from './CodeBlock'
import { isImagePath, mediaUrl } from '../../lib/paths'

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

const isExternal = (href: string) => /^https?:\/\//i.test(href)

// Markdown treats backslashes as escapes, which mangles Windows paths inside
// link/image targets (![x](C:\a\b.png)). Normalise backslashes to forward
// slashes within ](...) targets so local paths survive parsing.
function normalizeWindowsPaths(src: string): string {
  return src.replace(/(!?\]\()([^)]+)(\))/g, (_m, a, target, c) => a + target.replace(/\\/g, '/') + c)
}

// Markdown renders assistant content as GitHub-flavored markdown: headings,
// lists, tables, task lists, fenced code with syntax highlight + diffs, inline
// images (local paths resolved through /api/files) and clickable links.
function MarkdownImpl({ children, onOpenFile }: Props) {
  return (
    <div className="sg-markdown text-sm leading-relaxed">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        // Disable the default URL sanitiser: it treats "C:" as an unknown
        // protocol and strips local Windows paths. We resolve links/images
        // ourselves in the components below.
        urlTransform={(u) => u}
        components={{
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
            if (url && !isExternal(url) && !url.startsWith('#') && !url.startsWith('mailto:')) {
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
            // Resolve local image paths through the backend file server.
            const resolved = isExternal(raw) || raw.startsWith('data:')
              ? raw
              : isImagePath(raw)
                ? mediaUrl(raw)
                : raw
            return (
              <img
                src={resolved}
                alt={alt || ''}
                className="my-2 max-h-96 rounded-lg border border-[var(--color-border)]"
              />
            )
          },
        }}
      >
        {normalizeWindowsPaths(children)}
      </ReactMarkdown>
    </div>
  )
}

export const Markdown = memo(MarkdownImpl)
