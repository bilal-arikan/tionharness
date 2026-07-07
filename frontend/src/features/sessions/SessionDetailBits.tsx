import { Loader2, ChevronDown, type LucideIcon } from 'lucide-react'

// ---- presentational helpers ----

export function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section>
      <div className="mb-2 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] opacity-70">
        {title}
      </div>
      {children}
    </section>
  )
}

// SaveRow is one savings line in the session spend card: a label, an optional
// hint, and a green value (USD or bytes).
export function SaveRow({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div className="flex items-center justify-between text-[11px]">
      <span className="text-[var(--color-text-dim)]">
        {label}
        {hint && <span className="ml-1 opacity-60">· {hint}</span>}
      </span>
      <span className="ml-2 shrink-0 font-medium" style={{ color: 'var(--color-success)' }}>
        {value}
      </span>
    </div>
  )
}

// ProcBtn is a compact control in the background-process card (stop/restart/drop).
export function ProcBtn({
  icon: Icon,
  label,
  onClick,
  busy,
  disabled,
  title,
}: {
  icon: LucideIcon
  label: string
  onClick: () => void
  busy?: boolean
  disabled?: boolean
  title?: string
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      title={title ?? label}
      className="flex items-center gap-1 rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-[11px] text-[var(--color-text)] transition hover:border-[var(--color-accent)] disabled:opacity-40"
    >
      {busy ? <Loader2 size={12} className="shrink-0 animate-spin" /> : <Icon size={12} className="shrink-0" />}
      {label}
    </button>
  )
}

export function Pill({ children, accent }: { children: React.ReactNode; accent?: boolean }) {
  return (
    <span
      className={`rounded-full px-2 py-0.5 text-[10px] ${
        accent
          ? 'bg-[var(--color-accent)]/15 text-[var(--color-accent)]'
          : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
      }`}
    >
      {children}
    </span>
  )
}

export function ActionBtn({
  icon: Icon,
  label,
  onClick,
  disabled,
  danger,
  caret,
  busy,
}: {
  icon: LucideIcon
  label: string
  onClick: () => void
  disabled?: boolean
  danger?: boolean
  caret?: boolean
  busy?: boolean
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className={`flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-xs transition disabled:opacity-30 ${
        danger
          ? 'text-[var(--color-danger)] hover:bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)]'
          : 'text-[var(--color-text)] hover:bg-[var(--color-surface-2)]'
      }`}
    >
      {busy ? <Loader2 size={14} className="shrink-0 animate-spin" /> : <Icon size={14} className="shrink-0" />}
      <span className="flex-1">{label}</span>
      {caret && <ChevronDown size={14} className="text-[var(--color-text-dim)]" />}
    </button>
  )
}
