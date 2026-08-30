import { displayPath, splitPaths, urlHref } from '@/shared/lib/paths'

interface Props {
  text: string
  onOpenFile?: (path: string) => void
}

// PathText renders plain text with embedded file paths turned into clickable,
// monospace chips and embedded URLs turned into real links — used for tool
// summaries (WebSearch/WebFetch show their URL here) and other non-markdown
// strings.
export function PathText({ text, onOpenFile }: Props) {
  const segments = splitPaths(text)
  return (
    <>
      {segments.map((seg, i) =>
        seg.kind === 'url' ? (
          // stopPropagation: following the link must not also toggle the card.
          <a
            key={i}
            href={urlHref(seg.text)}
            target="_blank"
            rel="noreferrer"
            onClick={(e) => e.stopPropagation()}
            className="break-all text-[var(--color-accent)] underline underline-offset-2 hover:opacity-80"
          >
            {seg.text}
          </a>
        ) : seg.kind === 'path' ? (
          // A span (not a <button>) so it stays valid HTML when PathText sits
          // inside a clickable card header (which is itself a <button>) —
          // nested buttons are invalid and trigger a hydration warning.
          // stopPropagation: clicking a path opens it without toggling the card.
          <span
            key={i}
            role="button"
            tabIndex={0}
            onClick={(e) => {
              e.stopPropagation()
              onOpenFile?.(seg.target || seg.text)
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                e.stopPropagation()
                onOpenFile?.(seg.target || seg.text)
              }
            }}
            className="cursor-pointer break-all font-mono text-[0.92em] text-[var(--color-accent)] underline decoration-dotted underline-offset-2 hover:opacity-80"
          >
            {displayPath(seg.text)}
          </span>
        ) : (
          <span key={i}>{seg.text}</span>
        ),
      )}
    </>
  )
}
