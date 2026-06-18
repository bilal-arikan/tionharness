import { displayPath, splitPaths } from '../../lib/paths'

interface Props {
  text: string
  onOpenFile?: (path: string) => void
}

// PathText renders plain text with embedded file paths turned into clickable,
// monospace chips — used for tool summaries and other non-markdown strings.
export function PathText({ text, onOpenFile }: Props) {
  const segments = splitPaths(text)
  return (
    <>
      {segments.map((seg, i) =>
        seg.isPath ? (
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
              onOpenFile?.(seg.text)
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                e.stopPropagation()
                onOpenFile?.(seg.text)
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
