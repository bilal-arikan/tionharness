import { useEffect, useState } from 'react'
import { Markdown } from '@/shared/components/markdown/Markdown'
import { CodeBlock } from '@/shared/components/markdown/CodeBlock'

// Extensions a `file` artifact can be previewed inline as text. Anything else
// (pdf, zip, binaries) keeps the plain download card.
const MAX_INLINE_BYTES = 2 * 1024 * 1024

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
