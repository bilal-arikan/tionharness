import { useEffect, useState } from 'react'
import { useKeyedReset } from '@/shared/lib/useKeyedReset'
import { Markdown } from '@/shared/components/markdown/Markdown'
import { CodeBlock } from '@/shared/components/markdown/CodeBlock'
import { useTranslation } from 'react-i18next'

// Extensions a `file` artifact can be previewed inline as text. Anything else
// (pdf, zip, binaries) keeps the plain download card.
const MAX_INLINE_BYTES = 2 * 1024 * 1024

export function TextFileArtifact({ url, lang }: { url: string; lang: string }) {
  const { t } = useTranslation('artifacts')
  const [text, setText] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  // A new file drops the previous preview before the fetch below lands.
  useKeyedReset(url, () => {
    setText(null)
    setError(null)
  })
  useEffect(() => {
    let alive = true
    fetch(url)
      .then(async (res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        const body = await res.text()
        if (body.length > MAX_INLINE_BYTES) throw new Error(t('preview.fileTooLarge'))
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
  }, [url, t])

  if (error) {
    return (
      <div className="text-xs text-[var(--color-text-dim)]">{t('preview.failed', { error })}</div>
    )
  }
  if (text === null) {
    return <div className="text-xs text-[var(--color-text-dim)]">{t('preview.loading')}</div>
  }
  if (lang === 'markdown') return <Markdown>{text}</Markdown>
  return <CodeBlock code={text} lang={lang || undefined} />
}
