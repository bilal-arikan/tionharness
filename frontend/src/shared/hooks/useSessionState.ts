import { useCallback, useState, type Dispatch, type SetStateAction } from 'react'

// Module-level, in-memory store. Persists for the lifetime of the loaded page
// (survives component unmount/remount as the user navigates between screens) but
// is NOT written to localStorage, so a full app reload / reopen starts fresh.
const store = new Map<string, unknown>()

// useSessionState is a drop-in replacement for useState whose value is remembered
// across unmounts within the same page session, keyed by a stable string. Each
// screen uses it for its "selected item" so switching screens and coming back
// keeps the selection, while closing/reopening the app resets it.
//
// Keys must be unique per logical selection (e.g. "artifacts.activeId"). The
// `initial` is used only when the key has never been written this session.
export function useSessionState<T>(key: string, initial: T): [T, Dispatch<SetStateAction<T>>] {
  const [value, setValue] = useState<T>(() => (store.has(key) ? (store.get(key) as T) : initial))

  const set = useCallback<Dispatch<SetStateAction<T>>>(
    (next) => {
      setValue((prev) => {
        const resolved = typeof next === 'function' ? (next as (p: T) => T)(prev) : next
        store.set(key, resolved)
        return resolved
      })
    },
    [key],
  )

  return [value, set]
}

// setSessionState seeds a selection from OUTSIDE the component that owns it, so
// one screen can deep-link into another's selected item before navigating there
// (e.g. the session inspector opening a coordinator workflow's skill). The target
// screen is mounted on view switch and reads the seeded value in its initialiser,
// so this must be called BEFORE the navigation that mounts it.
export function setSessionState<T>(key: string, value: T): void {
  store.set(key, value)
}

// clearSessionState wipes one or all session selections (e.g. on explicit reset).
export function clearSessionState(key?: string): void {
  if (key === undefined) store.clear()
  else store.delete(key)
}
