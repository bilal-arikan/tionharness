import { Workflow } from 'lucide-react'
import type { Flow } from '@/types'
import { normalizeAvatar } from '@/shared/lib/avatar'

// TargetModeToggle is a small segmented control letting a schedule/automation
// target either a single agent or an orchestration flow.
export function TargetModeToggle({
  mode,
  onChange,
}: {
  mode: 'agent' | 'flow'
  onChange: (m: 'agent' | 'flow') => void
}) {
  return (
    <div className="inline-flex overflow-hidden rounded border border-[var(--color-border)] text-xs">
      {(['agent', 'flow'] as const).map((m) => (
        <button
          key={m}
          type="button"
          onClick={() => onChange(m)}
          className={`px-2 py-1 transition ${
            mode === m
              ? 'bg-[var(--color-accent)] text-white'
              : 'bg-[var(--color-bg)] text-[var(--color-text-dim)] hover:text-[var(--color-accent)]'
          }`}
        >
          {m === 'agent' ? 'Ajan' : 'Akış'}
        </button>
      ))}
    </div>
  )
}

// FlowPicker is a simple dropdown of the workspace flows (mirrors AgentPicker).
export function FlowPicker({
  flows,
  value,
  onChange,
}: {
  flows: Flow[]
  value: string
  onChange: (id: string) => void
}) {
  // Selected flow's icon (mojibake-safe emoji, or a Workflow glyph fallback),
  // shown next to the dropdown — mirrors the AgentPicker's leading avatar.
  const selected = flows.find((f) => f.id === value)
  const selectedEmoji = normalizeAvatar(selected?.emoji)
  return (
    <div className="flex items-center gap-1.5 rounded border border-[var(--color-border)] bg-[var(--color-bg)] pl-1.5 focus-within:border-[var(--color-accent)]">
      <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded bg-[var(--color-accent-soft)] text-[var(--color-accent)]">
        {selectedEmoji ? <span className="text-sm leading-none">{selectedEmoji}</span> : <Workflow size={13} />}
      </span>
      <select
        data-testid="flow-picker"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="rounded bg-transparent py-1 pr-2 text-sm outline-none"
      >
        <option value="">Akış seç…</option>
        {flows.map((f) => (
          <option key={f.id} value={f.id}>
            {normalizeAvatar(f.emoji) ? `${normalizeAvatar(f.emoji)} ${f.name}` : f.name}
          </option>
        ))}
      </select>
    </div>
  )
}

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
