import { Hash } from 'lucide-react'
import { AgentAvatar } from '../../agents/AgentAvatar'
import type { MenuItem, Trigger } from './trigger'

interface Props {
  // The open trigger; its mode picks the menu heading. Never null when rendered.
  mode: NonNullable<Trigger>['mode']
  items: MenuItem[]
  sel: number
  onHover: (index: number) => void
  onChoose: (index: number) => void
}

const HEADINGS: Record<Props['mode'], string> = {
  agent: 'Ajanlar (referans)',
  artifact: 'Artifactlar',
  command: 'Komutlar',
}

// AutocompleteMenu is the "@/#//" picker dropdown anchored above the composer
// input. Each row shows an agent avatar (name reference), an artifact hash, or a
// command glyph.
export function AutocompleteMenu({ mode, items, sel, onHover, onChoose }: Props) {
  return (
    <div className="absolute bottom-full left-6 mb-2 max-h-64 w-80 overflow-y-auto rounded-xl border border-[var(--color-border)] bg-[var(--color-surface-2)] p-1 shadow-xl">
      <div className="px-2 py-1 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
        {HEADINGS[mode]}
      </div>
      {items.map((it, i) => (
        <button
          key={it.key}
          onMouseEnter={() => onHover(i)}
          onClick={() => onChoose(i)}
          className={`flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm ${
            i === sel
              ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]'
              : 'text-[var(--color-text-dim)]'
          }`}
        >
          {it.agent ? (
            <AgentAvatar agent={it.agent} size={22} />
          ) : it.artifact ? (
            <span className="flex h-[22px] w-[22px] shrink-0 items-center justify-center text-[var(--color-accent)]">
              <Hash size={15} />
            </span>
          ) : (
            <span className="flex h-[22px] w-[22px] shrink-0 items-center justify-center">
              {it.cmd?.icon ?? '⚡'}
            </span>
          )}
          <span className="flex min-w-0 flex-col">
            <span className="truncate font-medium text-[var(--color-text)]">{it.label}</span>
            {it.sub && <span className="truncate text-xs opacity-70">{it.sub}</span>}
          </span>
        </button>
      ))}
    </div>
  )
}
