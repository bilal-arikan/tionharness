// Flow-run status label + colour. Split out so RunView.tsx exports only
// components (fast refresh).
import { i18next } from '@/i18n'

export function statusLabel(status: string): string {
  return i18next.t(`status.${status}`, { ns: 'flows', defaultValue: status })
}

export function statusColor(status: string): string {
  if (status === 'success') return 'text-[var(--color-success)]'
  if (status === 'failure') return 'text-[var(--color-danger)]'
  if (status === 'waiting') return 'text-[var(--color-warning)]'
  return 'text-[var(--color-accent)]'
}
