import { useCallback, useEffect, useRef, useState } from 'react'

// Per-session composer draft persistence. A half-written, unsent message is kept
// in localStorage keyed by session id, so it survives switching to another
// session (and back) and a full page reload. Sending or clearing the composer
// removes the draft; an empty draft is never stored.

const PREFIX = 'swarmgo:draft:'

function read(sessionId?: string): string {
  if (!sessionId) return ''
  try {
    return localStorage.getItem(PREFIX + sessionId) ?? ''
  } catch {
    return ''
  }
}

function write(sessionId: string | undefined, text: string) {
  if (!sessionId) return
  try {
    if (text) localStorage.setItem(PREFIX + sessionId, text)
    else localStorage.removeItem(PREFIX + sessionId)
  } catch {
    // Ignore quota / private-mode write failures — drafts are best-effort.
  }
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
