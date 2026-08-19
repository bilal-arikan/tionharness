import { useCallback, useEffect, useRef, useState } from 'react'
import { readSessionDraft, writeSessionDraft } from '@/shared/lib/sessionDrafts'

// Composer-side hook over the per-session draft store (shared/lib/sessionDrafts).
// The storage itself lives there because the sessions sidebar also reads it, to
// mark rows whose composer has unsent text.

// writeSessionDraft persists a draft for a session from outside the hook (e.g.
// restoring a rewound prompt into the composer). The Composer re-reads the draft
// on mount, so the caller should remount it (key bump) after calling this.
export { writeSessionDraft } from '@/shared/lib/sessionDrafts'

// useSessionDraft returns the current session's draft text and a setter that
// persists every edit immediately. Switching sessionId loads that session's own
// draft (the previous session's text was already saved on each keystroke).
export function useSessionDraft(sessionId?: string): [string, (v: string) => void] {
  const [text, setText] = useState(() => readSessionDraft(sessionId))
  // Tracks the session the current text belongs to, so the setter always writes
  // to the right key even before the change effect runs.
  const current = useRef(sessionId)

  useEffect(() => {
    if (current.current === sessionId) return
    current.current = sessionId
    setText(readSessionDraft(sessionId))
  }, [sessionId])

  const setDraft = useCallback((v: string) => {
    setText(v)
    writeSessionDraft(current.current, v)
  }, [])

  return [text, setDraft]
}
