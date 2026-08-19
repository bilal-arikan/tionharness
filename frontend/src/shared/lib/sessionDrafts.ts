// Per-session composer draft storage, plus a tiny observable index of which
// sessions currently hold an unsent draft.
//
// A half-written, unsent message is kept in localStorage keyed by workspace +
// session id, so it survives switching to another session (and back) and a full
// page reload. Sending or clearing the composer removes the draft; an empty draft
// is never stored.
//
// The workspace is part of the key because session ids are per-workspace
// sequences: "SES1" is the first session of EVERY workspace, so a bare-id key
// showed one workspace's unsent draft in another workspace's composer.
//
// The observable part exists for the sessions sidebar: it marks rows whose
// composer has text waiting. localStorage fires no event in the writing tab, so
// every write goes through write() below and notifies subscribers directly; the
// `storage` event covers other tabs.

import { getActiveWorkspace } from '@/api'

const PREFIX = 'tionswarm:draft:'

// draftKey scopes a session's draft to the ACTIVE workspace. Read at call time
// (not captured): the composer remounts on a workspace switch, so each mount
// resolves the key for the workspace it is rendering.
function draftKey(sessionId?: string): string | null {
  if (!sessionId) return null
  const ws = getActiveWorkspace()
  if (!ws) return null
  return PREFIX + ws + ':' + sessionId
}

export function readSessionDraft(sessionId?: string): string {
  const key = draftKey(sessionId)
  if (!key) return ''
  try {
    return localStorage.getItem(key) ?? ''
  } catch {
    return ''
  }
}

export function writeSessionDraft(sessionId: string | undefined, text: string) {
  const key = draftKey(sessionId)
  if (!key) return
  try {
    if (text) localStorage.setItem(key, text)
    else localStorage.removeItem(key)
  } catch {
    // Ignore quota / private-mode write failures — drafts are best-effort.
  }
  notify()
}

// draftSessionIds lists the sessions of the ACTIVE workspace that hold a
// non-empty draft right now. Derived from storage on demand (drafts are few and
// small) rather than mirrored in a second source of truth that could drift.
export function draftSessionIds(): Set<string> {
  const ids = new Set<string>()
  const ws = getActiveWorkspace()
  if (!ws) return ids
  const scope = PREFIX + ws + ':'
  try {
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i)
      if (!key || !key.startsWith(scope)) continue
      if (localStorage.getItem(key)) ids.add(key.slice(scope.length))
    }
  } catch {
    // Storage unavailable: no draft markers, same as having none.
  }
  return ids
}

const listeners = new Set<() => void>()

function notify() {
  for (const fn of listeners) fn()
}

// subscribeSessionDrafts registers a callback fired whenever the draft set may
// have changed (a write in this tab, or any storage write from another tab).
// Returns the unsubscribe function.
export function subscribeSessionDrafts(fn: () => void): () => void {
  listeners.add(fn)
  if (listeners.size === 1) window.addEventListener('storage', notify)
  return () => {
    listeners.delete(fn)
    if (listeners.size === 0) window.removeEventListener('storage', notify)
  }
}
