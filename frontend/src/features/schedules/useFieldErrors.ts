import { useState } from 'react'

// useFieldErrors is the shared form-validation helper for the automation and
// schedule modals. The caller computes a record of fieldKey → error message
// (empty string = valid) on every render; this hook tracks whether the user has
// attempted a submit yet (so inline messages stay hidden until then), exposes the
// first blocking error (in the record's insertion order — that IS the priority),
// and gates each field's message behind the attempt.
export function useFieldErrors<K extends string>(errors: Record<K, string>) {
  const [attempted, setAttempted] = useState(false)
  // Object.values preserves string-key insertion order, so the record order is
  // the error priority the caller intends.
  const firstError = Object.values(errors).find((e) => e) as string | undefined
  return {
    attempted,
    markAttempted: () => setAttempted(true),
    // The first blocking error, or undefined when every field is valid.
    firstError,
    // A field's message, but only after a submit attempt (empty before that).
    errorFor: (key: K): string => (attempted ? errors[key] : ''),
  }
}

// FieldError renders a single inline validation message under a form field, or
// nothing when there is no error. Kept next to the hook so both modals render
// their errors identically.
