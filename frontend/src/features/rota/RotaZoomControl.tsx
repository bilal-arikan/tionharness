// Zoom readout + steppers for the Rota canvas. Kept next to the window filter
// in the header; the canvas itself handles ctrl/⌘-wheel zooming.
import { Minus, Plus } from 'lucide-react'
import { MAX_ZOOM, MIN_ZOOM, formatZoom, stepZoom } from './rotaZoom'

interface Props {
  zoom: number
  onZoom: (z: number) => void
}

export function RotaZoomControl({ zoom, onZoom }: Props) {
  const btn =
    'rounded px-1 py-0.5 text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)] disabled:opacity-30 disabled:hover:bg-transparent'
  return (
    <span
      className="flex items-center gap-0.5"
      title="Zaman eksenini yakınlaştır (Ctrl + tekerlek)"
    >
      <span className="text-[var(--color-text-dim)]">yakınlık</span>
      <button
        type="button"
        onClick={() => onZoom(stepZoom(zoom, -1))}
        disabled={zoom <= MIN_ZOOM}
        className={btn}
        aria-label="Uzaklaştır"
      >
        <Minus size={12} />
      </button>
      <button
        type="button"
        onClick={() => onZoom(MIN_ZOOM)}
        className="min-w-[2.2rem] rounded px-1 py-0.5 text-center tabular-nums hover:bg-[var(--color-surface-2)]"
        title="Panele sığdır"
      >
        {formatZoom(zoom)}
      </button>
      <button
        type="button"
        onClick={() => onZoom(stepZoom(zoom, 1))}
        disabled={zoom >= MAX_ZOOM}
        className={btn}
        aria-label="Yakınlaştır"
      >
        <Plus size={12} />
      </button>
    </span>
  )
}
