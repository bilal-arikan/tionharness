import { splitPaths } from '../../lib/paths'

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
          <button
            key={i}
            type="button"
            onClick={() => onOpenFile?.(seg.text)}
            className="break-all font-mono text-[0.92em] text-[var(--color-accent)] underline decoration-dotted underline-offset-2 hover:opacity-80"
          >
            {seg.text}
          </button>
        ) : (
          <span key={i}>{seg.text}</span>
        ),
      )}
    </>
  )
}
