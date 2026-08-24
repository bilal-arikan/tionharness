// Flow-run status label + colour. Split out so RunView.tsx exports only
// components (fast refresh).
export const STATUS_LABEL: Record<string, string> = {
  running: '▶ devam ediyor',
  success: '✓ başarılı',
  failure: '✕ hata',
  waiting: '⏳ girdi bekleniyor',
}

export function statusColor(status: string): string {
  if (status === 'success') return 'text-[var(--color-success)]'
  if (status === 'failure') return 'text-[var(--color-danger)]'
  if (status === 'waiting') return 'text-[var(--color-warning)]'
  return 'text-[var(--color-accent)]'
}
