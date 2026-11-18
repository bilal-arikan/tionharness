// navItemClass renders the shared look for a nav button. The active state is a
// soft accent tint with an accent left indicator (instead of a heavy solid
// fill), which reads as more modern and less visually loud.
export function navItemClass(active: boolean, collapsed: boolean): string {
  return [
    'group relative flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition',
    collapsed ? 'justify-center' : '',
    active
      ? 'bg-[var(--color-accent-soft)] font-medium text-[var(--color-accent)]'
      : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]',
  ].join(' ')
}
