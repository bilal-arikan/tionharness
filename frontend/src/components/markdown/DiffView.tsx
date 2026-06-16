import { parseDiff } from '../../lib/diff'

interface Props {
  text: string
}

const ROW: Record<string, string> = {
  add: 'bg-green-500/12 text-green-300',
  del: 'bg-red-500/12 text-red-300',
  hunk: 'bg-[var(--color-accent)]/10 text-[var(--color-accent)]',
  meta: 'text-[var(--color-text-dim)]',
  ctx: 'text-[var(--color-text)]',
}

const GUTTER: Record<string, string> = {
  add: '+',
  del: '-',
  hunk: ' ',
  meta: ' ',
  ctx: ' ',
}

// DiffView renders a unified diff with per-line +/- coloring and a small
// add/remove stat header, matching the file-change cards in External Agent chat.
export function DiffView({ text }: Props) {
  const { lines, stats } = parseDiff(text)

  return (
    <div className="overflow-hidden rounded-lg border border-[var(--color-border)]">
      <div className="flex items-center gap-3 border-b border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-xs">
        <span className="text-green-400">+{stats.added}</span>
        <span className="text-red-400">−{stats.removed}</span>
      </div>
      <pre className="overflow-x-auto bg-[var(--color-bg)] py-1 text-xs leading-relaxed">
        <code className="block font-mono">
          {lines.map((l, i) => (
            <div key={i} className={`flex px-3 ${ROW[l.kind]}`}>
              <span className="mr-2 select-none opacity-50">{GUTTER[l.kind]}</span>
              <span className="whitespace-pre-wrap break-all">
                {l.kind === 'add' || l.kind === 'del' ? l.text.slice(1) : l.text}
              </span>
            </div>
          ))}
        </code>
      </pre>
    </div>
  )
}
