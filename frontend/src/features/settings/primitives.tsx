// Shared building blocks for the settings screen: field primitives, the category
// rail button, the read-only prompt viewer and the category taxonomy.
import type { ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'
import type { AppSettings, PromptInfo, WorkspaceSettings } from '@/types'
import { CopyPathButton } from '@/shared/components/CopyPathButton'

// Category keys: the app-global sections plus the per-workspace section.
export type Cat =
  | 'profile'
  | 'appearance'
  | 'providers'
  | 'secrets'
  | 'context'
  | 'tools'
  | 'hooks'
  | 'exttools'
  | 'sound'
  | 'advanced'
  | 'backup'
  | 'commands'
  | 'stepkinds'
  | 'about'
  | 'workspace'
  | 'wsfiles'

export interface CatMeta {
  key: Cat
  label: string
  icon: LucideIcon
}

export type AppSet = <K extends keyof AppSettings>(key: K, val: AppSettings[K]) => void
export type WsSet = <K extends keyof WorkspaceSettings>(key: K, val: WorkspaceSettings[K]) => void

export const inputCls =
  'rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]'

export function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: ReactNode
}) {
  return (
    <label className="flex flex-col gap-1">
      <span className="text-sm font-medium">{label}</span>
      {children}
      {hint && <span className="text-xs text-[var(--color-text-dim)]">{hint}</span>}
    </label>
  )
}

export function Toggle({
  label,
  hint,
  checked,
  onChange,
}: {
  label: string
  hint?: string
  checked: boolean
  onChange: (v: boolean) => void
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      onClick={() => onChange(!checked)}
      className="flex items-center justify-between gap-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-left"
    >
      <span>
        <span className="block text-sm font-medium">{label}</span>
        {hint && <span className="block text-xs text-[var(--color-text-dim)]">{hint}</span>}
      </span>
      <span
        className={`relative h-5 w-9 flex-shrink-0 rounded-full transition ${
          checked ? 'bg-[var(--color-accent)]' : 'bg-[var(--color-surface-2)]'
        }`}
      >
        <span
          className={`absolute top-0.5 h-4 w-4 rounded-full bg-[var(--color-text)] transition-all ${
            checked ? 'left-4' : 'left-0.5'
          }`}
        />
      </span>
    </button>
  )
}

// Segmented is a single-choice control (radio-as-buttons) for a small set of
// mutually-exclusive options. Preferred over multiple booleans when the choices
// exclude each other — it makes the "only one" contract visual and removes the need
// for a "both on → which wins?" warning. The active option's own hint is shown when
// present, so the description updates with the selection.
export function Segmented<T extends string>({
  label,
  hint,
  value,
  options,
  onChange,
}: {
  label: string
  hint?: string
  value: T
  options: { value: T; label: string; hint?: string }[]
  onChange: (v: T) => void
}) {
  const active = options.find((o) => o.value === value)
  return (
    <div className="flex flex-col gap-1 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2">
      <span className="text-sm font-medium">{label}</span>
      <div className="mt-0.5 flex gap-0.5 rounded-md bg-[var(--color-surface-2)] p-0.5">
        {options.map((o) => (
          <button
            key={o.value}
            onClick={() => onChange(o.value)}
            className={`flex-1 rounded px-2 py-1 text-xs font-medium transition ${
              o.value === value
                ? 'bg-[var(--color-accent)] text-[var(--color-on-accent)]'
                : 'text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
            }`}
          >
            {o.label}
          </button>
        ))}
      </div>
      {(active?.hint ?? hint) && (
        <span className="text-xs text-[var(--color-text-dim)]">{active?.hint ?? hint}</span>
      )}
    </div>
  )
}

// Slider is a labelled range input with a live value badge and an optional
// sub-line (e.g. a derived/absolute figure). Used where a raw 0–1 number is
// unintuitive — the badge and sub turn it into a readable control.
export function Slider({
  label,
  hint,
  sub,
  value,
  onChange,
  min = 0,
  max = 1,
  step = 0.05,
  badge,
  disabled,
}: {
  label: string
  hint?: string
  sub?: ReactNode
  value: number
  onChange: (v: number) => void
  min?: number
  max?: number
  step?: number
  badge?: string
  disabled?: boolean
}) {
  return (
    <div className={`flex flex-col gap-1 ${disabled ? 'opacity-50' : ''}`}>
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium">{label}</span>
        {badge && (
          <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-xs font-medium tabular-nums text-[var(--color-text)]">
            {badge}
          </span>
        )}
      </div>
      <input
        type="range"
        min={min}
        max={max}
        step={step}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        className="h-1.5 w-full cursor-pointer appearance-none rounded-full bg-[var(--color-surface-2)] accent-[var(--color-accent)]"
      />
      {hint && <span className="text-xs text-[var(--color-text-dim)]">{hint}</span>}
      {sub && <span className="text-xs text-[var(--color-text-dim)]">{sub}</span>}
    </div>
  )
}

export function CatButton({
  c,
  active,
  onClick,
  dirty,
}: {
  c: CatMeta
  active: boolean
  onClick: () => void
  dirty: boolean
}) {
  return (
    <button
      onClick={onClick}
      className={`flex items-center gap-2 rounded-lg px-3 py-2 text-left text-sm transition ${
        active
          ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]'
          : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'
      }`}
    >
      <c.icon size={16} className="shrink-0" />
      <span className="flex-1 truncate">{c.label}</span>
      {dirty && (
        <span className="h-1.5 w-1.5 rounded-full bg-[var(--color-accent)]" title="Kaydedilmemiş" />
      )}
    </button>
  )
}

// PromptDetails renders one built-in prompt read-only (System / User-turn blocks
// + note) with a button to copy the source folder path. Used inside the
// expandable command cards on the Komutlar screen.
export function PromptDetails({ p, dir }: { p: PromptInfo; dir: string }) {
  return (
    <>
      <div className="mb-2 flex items-center justify-between gap-2">
        <code className="rounded bg-[var(--color-surface-2)] px-1 text-[10px] text-[var(--color-text-dim)]">
          {p.file}
        </code>
        <div className="flex shrink-0 items-center gap-1">
          <CopyPathButton path={dir} />
        </div>
      </div>
      {p.system && (
        <div className="mb-2">
          <div className="text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
            System
          </div>
          <pre className="mt-0.5 max-h-40 overflow-auto whitespace-pre-wrap rounded bg-[var(--color-surface-2)] p-2 text-xs text-[var(--color-text)]">
            {p.system}
          </pre>
        </div>
      )}
      {p.user && (
        <div className="mb-2">
          <div className="text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
            User turn
          </div>
          <pre className="mt-0.5 max-h-40 overflow-auto whitespace-pre-wrap rounded bg-[var(--color-surface-2)] p-2 text-xs text-[var(--color-text)]">
            {p.user}
          </pre>
        </div>
      )}
      {p.note && <p className="text-xs text-[var(--color-text-dim)]">{p.note}</p>}
    </>
  )
}
