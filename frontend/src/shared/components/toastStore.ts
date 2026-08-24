// Toast state + the imperative `toast.*` API. Kept apart from the <Toaster/> UI so
// that module stays component-only (fast refresh).
export type ToastTone = 'error' | 'success' | 'info'

export interface ToastItem {
  id: number
  message: string
  tone: ToastTone
  ttl: number
}

type Listener = (items: ToastItem[]) => void

let items: ToastItem[] = []
let seq = 0
const listeners = new Set<Listener>()

function emit() {
  for (const l of listeners) l(items)
}

function push(message: string, tone: ToastTone, ttl: number): number | undefined {
  if (!message) return undefined
  const id = ++seq
  items = [...items, { id, message, tone, ttl }]
  emit()
  return id
}

// subscribe pushes the current list immediately, then on every change. Returns the
// unsubscribe so <Toaster/> never has to reach into the listener set itself.
export function subscribe(fn: Listener): () => void {
  listeners.add(fn)
  fn(items)
  return () => {
    listeners.delete(fn)
  }
}

export function dismiss(id: number) {
  items = items.filter((t) => t.id !== id)
  emit()
}

// Public API. Errors linger longer (8s) than positive/neutral notices (4s).
export const toast = {
  error: (message: string, ttl = 8000) => push(message, 'error', ttl),
  success: (message: string, ttl = 4000) => push(message, 'success', ttl),
  info: (message: string, ttl = 4000) => push(message, 'info', ttl),
  dismiss,
}

// Per-tone icon + full (JIT-safe literal) className strings. The colours are
// mixed from the theme's semantic tokens, so they re-theme with presets and the
// light theme automatically.
