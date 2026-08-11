import { useEffect, useState } from 'react'
import { Markdown } from '@/shared/components/markdown/Markdown'
import { CodeBlock } from '@/shared/components/markdown/CodeBlock'

// Extensions a `file` artifact can be previewed inline as text. Anything else
// (pdf, zip, binaries) keeps the plain download card.
const TEXT_EXT: Record<string, string> = {
  md: 'markdown',
  markdown: 'markdown',
  txt: '',
  log: '',
  json: 'json',
  yaml: 'yaml',
  yml: 'yaml',
  csv: '',
  toml: 'toml',
  ts: 'typescript',
  tsx: 'tsx',
  js: 'javascript',
  go: 'go',
  py: 'python',
  sh: 'bash',
  sql: 'sql',
}

// Max bytes rendered inline; larger files stay a download-only card so the
// artifacts screen can't be wedged by a huge log.
const MAX_INLINE_BYTES = 2 * 1024 * 1024

export function textFileLang(sourcePath: string): string | undefined {
  const ext = sourcePath.split('.').pop()?.toLowerCase() ?? ''
  return ext in TEXT_EXT ? TEXT_EXT[ext] : undefined
}

// TextFileArtifact fetches a stored text file and renders it inline: markdown
// files as prose, everything else as a syntax-highlighted block.
export function TextFileArtifact({ url, lang }: { url: string; lang: string }) {
  const [text, setText] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let alive = true
    setText(null)
    setError(null)
    fetch(url)
      .then(async (res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        const body = await res.text()
        if (body.length > MAX_INLINE_BYTES) throw new Error('file too large to preview')
        return body
      })
      .then((body) => {
        if (alive) setText(body)
      })
      .catch((err: unknown) => {
        if (alive) setError(err instanceof Error ? err.message : String(err))
      })
    return () => {
      alive = false
    }
  }, [url])

  if (error) {
    return <div className="text-xs text-[var(--color-text-dim)]">Önizlenemedi: {error}</div>
  }
  if (text === null) {
    return <div className="text-xs text-[var(--color-text-dim)]">Yükleniyor…</div>
  }
  if (lang === 'markdown') return <Markdown>{text}</Markdown>
  return <CodeBlock code={text} lang={lang || undefined} />
}
