// Rota toolbar: the idle-lane window. Live lanes always stay; idle ones drop
// out of the canvas once their last activity is older than the window.
export type IdleCutoff = 3600 | 21600 | 86400 | 0

const OPTIONS: { value: IdleCutoff; label: string }[] = [
  { value: 3600, label: '1 sa' },
  { value: 21600, label: '6 sa' },
  { value: 86400, label: '24 sa' },
  { value: 0, label: 'tümü' },
]

interface Props {
  cutoff: IdleCutoff
  onCutoff: (v: IdleCutoff) => void
}

export function RotaToolbar({ cutoff, onCutoff }: Props) {
  return (
    <span className="flex items-center gap-1" title="Boşta şeritleri bu pencerede tut">
      <span className="text-[var(--color-text-dim)]">pencere</span>
      {OPTIONS.map((o) => (
        <button
          key={o.value}
          type="button"
          onClick={() => onCutoff(o.value)}
          className={`rounded px-1.5 py-0.5 ${
            cutoff === o.value
              ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]'
              : 'text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
          }`}
        >
          {o.label}
        </button>
      ))}
    </span>
  )
}
