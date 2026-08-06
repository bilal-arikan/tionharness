import { useEffect, useState } from 'react'
import { AlertTriangle, CheckCircle2, Info, X, type LucideIcon } from 'lucide-react'

// Toast — the app-wide transient-message surface. Generalises the former
// error-only ErrorToast into a tone-aware stack (error / success / info).
//
// The store is module-level (not a React context) so `toast.*` is callable from
// anywhere — event handlers, api catch blocks, plain helpers — without threading
// a provider. A single <Toaster/> mounted at the app root subscribes and renders
// the stack pinned bottom-right; every existing `onError(msg)` sink now routes
// through `toast.error`, so error reporting stays backward-compatible.

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

function dismiss(id: number) {
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
const TONE: Record<
  ToastTone,
  { icon: LucideIcon; role: 'alert' | 'status'; box: string; btn: string }
> = {
  error: {
    icon: AlertTriangle,
    role: 'alert',
    box: 'border-[color-mix(in_srgb,var(--color-danger)_40%,transparent)] bg-[color-mix(in_srgb,var(--color-danger)_15%,var(--color-surface))] text-[var(--color-danger)]',
    btn: 'text-[var(--color-danger)] hover:bg-[color-mix(in_srgb,var(--color-danger)_20%,transparent)]',
  },
  success: {
    icon: CheckCircle2,
    role: 'status',
    box: 'border-[color-mix(in_srgb,var(--color-success)_40%,transparent)] bg-[color-mix(in_srgb,var(--color-success)_15%,var(--color-surface))] text-[var(--color-success)]',
    btn: 'text-[var(--color-success)] hover:bg-[color-mix(in_srgb,var(--color-success)_20%,transparent)]',
  },
  info: {
    icon: Info,
    role: 'status',
    box: 'border-[color-mix(in_srgb,var(--color-info)_40%,transparent)] bg-[color-mix(in_srgb,var(--color-info)_15%,var(--color-surface))] text-[var(--color-info)]',
    btn: 'text-[var(--color-info)] hover:bg-[color-mix(in_srgb,var(--color-info)_20%,transparent)]',
  },
}

function ToastRow({ item }: { item: ToastItem }) {
  useEffect(() => {
    if (!item.ttl) return
    const h = setTimeout(() => dismiss(item.id), item.ttl)
    return () => clearTimeout(h)
  }, [item.id, item.ttl])

  const t = TONE[item.tone]
  const Icon = t.icon
  return (
    <div
      role={t.role}
      className={`pointer-events-auto flex w-full items-start gap-2 rounded-lg border px-3 py-2 text-sm shadow-[var(--shadow-md)] ${t.box}`}
    >
      <Icon size={16} className="mt-0.5 shrink-0" />
      <span className="min-w-0 flex-1 break-words">{item.message}</span>
      <button
        onClick={() => dismiss(item.id)}
        title="Kapat"
        className={`shrink-0 rounded p-0.5 transition ${t.btn}`}
      >
        <X size={14} />
      </button>
    </div>
  )
}

// Toaster — mount once at the app root. Subscribes to the module store and
// renders the current stack. Newest toasts appear at the bottom of the stack.
export function Toaster() {
  const [list, setList] = useState<ToastItem[]>(items)
  useEffect(() => {
    listeners.add(setList)
    setList(items)
    return () => {
      listeners.delete(setList)
    }
  }, [])

  if (!list.length) return null
  return (
    <div className="pointer-events-none fixed bottom-4 right-4 z-50 flex w-[min(24rem,calc(100vw-2rem))] flex-col items-end gap-2">
      {list.map((t) => (
        <ToastRow key={t.id} item={t} />
      ))}
    </div>
  )
}
