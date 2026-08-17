import { useCallback, useEffect, useRef, useState } from 'react'
import { getActiveWorkspace } from '@/api'

// Per-session composer draft persistence. A half-written, unsent message is kept
// in localStorage keyed by workspace + session id, so it survives switching to
// another session (and back) and a full page reload. Sending or clearing the
// composer removes the draft; an empty draft is never stored.
//
// The workspace is part of the key because session ids are per-workspace
// sequences: "SES1" is the first session of EVERY workspace, so a bare-id key
// showed one workspace's unsent draft in another workspace's composer.

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

function read(sessionId?: string): string {
  const key = draftKey(sessionId)
  if (!key) return ''
  try {
    return localStorage.getItem(key) ?? ''
  } catch {
    return ''
  }
}

function write(sessionId: string | undefined, text: string) {
  const key = draftKey(sessionId)
  if (!key) return
  try {
    if (text) localStorage.setItem(key, text)
    else localStorage.removeItem(key)
  } catch {
    // Ignore quota / private-mode write failures — drafts are best-effort.
  }
}

// writeSessionDraft persists a draft for a session from outside the hook (e.g.
// restoring a rewound prompt into the composer). The Composer re-reads the draft
// on mount, so the caller should remount it (key bump) after calling this.
export function writeSessionDraft(sessionId: string | undefined, text: string) {
  write(sessionId, text)
}

// useSessionDraft returns the current session's draft text and a setter that
// persists every edit immediately. Switching sessionId loads that session's own
// draft (the previous session's text was already saved on each keystroke).
export function useSessionDraft(sessionId?: string): [string, (v: string) => void] {
  const [text, setText] = useState(() => read(sessionId))
  // Tracks the session the current text belongs to, so the setter always writes
  // to the right key even before the change effect runs.
  const current = useRef(sessionId)

  useEffect(() => {
    if (current.current === sessionId) return
    current.current = sessionId
    setText(read(sessionId))
  }, [sessionId])

  const setDraft = useCallback((v: string) => {
    setText(v)
    write(current.current, v)
  }, [])

  return [text, setDraft]
}
