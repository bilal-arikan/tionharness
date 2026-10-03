// CardAction is the prominent icon button used on board cards. Unlike the old
// bare icons it carries a border + surface fill so "run" and "edit" read as real
// buttons at a glance; `tone` colors the hover state.
export function CardAction({
  icon: Icon,
  label,
  tone = 'accent',
  disabled,
  onClick,
  testId,
  entityId,
}: {
  icon: React.ComponentType<{ size?: number }>
  label: string
  tone?: 'accent' | 'success'
  disabled?: boolean
  onClick: () => void
  testId?: string
  entityId?: string
}) {
  const hover =
    tone === 'success'
      ? 'hover:border-[var(--color-success)] hover:text-[var(--color-success)]'
      : 'hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]'
  return (
    <button
      type="button"
      data-testid={testId}
      data-schedule-id={entityId}
      onClick={onClick}
      disabled={disabled}
      title={label}
      aria-label={label}
      className={`flex h-7 w-7 items-center justify-center rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] text-[var(--color-text)] transition disabled:opacity-40 ${hover}`}
    >
      <Icon size={15} />
    </button>
  )
}

// Shared field chrome so the three modals look identical.
export const inputCls =
  'w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]'

export function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: React.ReactNode
}) {
  return (
    <label className="block" title={hint}>
      <span className="mb-1 block text-[11px] font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
        {label}
      </span>
      {children}
    </label>
  )
}
