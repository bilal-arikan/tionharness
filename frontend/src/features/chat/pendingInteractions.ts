import type { PendingAsk } from './AskPrompt'

export type PendingInteractions = Record<string, PendingAsk[]>

export function putInteraction(
  state: PendingInteractions,
  sid: string,
  ask: PendingAsk,
): PendingInteractions {
  const current = state[sid] ?? []
  const index = current.findIndex((item) => item.interactionId === ask.interactionId)
  const next = index < 0 ? [...current, ask] : current.map((item, i) => (i === index ? ask : item))
  return { ...state, [sid]: next }
}

export function removeInteraction(
  state: PendingInteractions,
  sid: string,
  id?: string,
): PendingInteractions {
  const current = state[sid]
  if (!current) return state
  const next = current.filter((ask) => ask.interactionId !== id)
  if (next.length === current.length) return state
  if (next.length) return { ...state, [sid]: next }
  const result = { ...state }
  delete result[sid]
  return result
}

// A normal reply or turn completion is not an answer to an asynchronous question.
export function retainAsyncInteractions(
  state: PendingInteractions,
  sid: string,
): PendingInteractions {
  const current = state[sid]
  if (!current) return state
  const next = current.filter((ask) => ask.async)
  if (next.length === current.length) return state
  if (next.length) return { ...state, [sid]: next }
  const result = { ...state }
  delete result[sid]
  return result
}
