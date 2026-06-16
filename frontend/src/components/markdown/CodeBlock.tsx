import { useMemo, useState } from 'react'
import hljs from 'highlight.js/lib/common'
import { DiffView } from './DiffView'
import { looksLikeDiff } from '../../lib/diff'

interface Props {
  code: string
  lang?: string
}

// CodeBlock renders a fenced code block with a language label, a copy button
// and highlight.js syntax coloring. `diff` blocks (explicit or detected) render
// through DiffView instead.
export function CodeBlock({ code, lang }: Props) {
  const [copied, setCopied] = useState(false)

  const isDiff = lang === 'diff' || (!lang && looksLikeDiff(code))

  const html = useMemo(() => {
    if (isDiff) return ''
    try {
      if (lang && hljs.getLanguage(lang)) {
        return hljs.highlight(code, { language: lang }).value
      }
      return hljs.highlightAuto(code).value
    } catch {
      return ''
    }
  }, [code, lang, isDiff])

  if (isDiff) return <DiffView text={code} />

  const copy = () => {
    navigator.clipboard.writeText(code).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1200)
    })
  }

  return (
    <div className="group relative my-2 overflow-hidden rounded-lg border border-[var(--color-border)]">
      <div className="flex items-center justify-between border-b border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1 text-[11px] text-[var(--color-text-dim)]">
        <span>{lang || 'text'}</span>
        <button
          onClick={copy}
          className="rounded px-1.5 py-0.5 opacity-0 transition hover:bg-[var(--color-surface-2)] group-hover:opacity-100"
        >
          {copied ? 'Copied' : 'Copy'}
        </button>
      </div>
      <pre className="overflow-x-auto bg-[var(--color-bg)] p-3 text-xs leading-relaxed">
        {html ? (
          <code className="hljs" dangerouslySetInnerHTML={{ __html: html }} />
        ) : (
          <code className="hljs">{code}</code>
        )}
      </pre>
    </div>
  )
}
