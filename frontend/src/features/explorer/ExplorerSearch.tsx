import { Search, X } from 'lucide-react'
export interface ExplorerSearchResult {
  key: string
  label: string
  // Already localized: the kind label, or the finer role label for sub nodes.
  kindLabel: string
  selected: boolean
}

interface Props {
  value: string
  onChange: (value: string) => void
  results: ExplorerSearchResult[]
  onPick: (key: string) => void
}

// In-map search: the canvas dims every non-matching node (focus + context) and
// this list is the fast path — one click selects and glides the camera to it.
export function ExplorerSearch({ value, onChange, results, onPick }: Props) {
  return (
    <div className="relative ml-2 hidden items-center sm:flex">
      <Search
        size={13}
        className="pointer-events-none absolute left-2 text-[var(--color-text-dim)]"
      />
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder="haritada ara…"
        aria-label="Haritada ara"
        className="w-44 rounded-md border border-[var(--color-border)] bg-transparent py-1 pl-7 pr-6 text-xs text-[var(--color-text)] focus:border-[var(--color-accent)] focus:outline-none"
      />
      {value && (
        <button
          onClick={() => onChange('')}
          title="Aramayı temizle"
          className="absolute right-1.5 text-[var(--color-text-dim)] hover:text-[var(--color-danger)]"
        >
          <X size={13} />
        </button>
      )}
      {value && results.length > 0 && (
        <div
          role="listbox"
          aria-label="Harita arama sonuçları"
          className="absolute left-0 top-8 z-30 max-h-80 w-72 overflow-y-auto rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-xl"
        >
          {results.map((result) => (
            <button
              key={result.key}
              role="option"
              aria-selected={result.selected}
              onClick={() => onPick(result.key)}
              className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs hover:bg-[var(--color-surface-2)]"
            >
              <span className="min-w-0 flex-1 truncate">{result.label}</span>
              <span className="shrink-0 text-[10px] text-[var(--color-text-dim)]">
                {result.kindLabel}
              </span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
