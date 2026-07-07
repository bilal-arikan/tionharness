// Failure lessons (self-healing, hata→ders döngüsü) — mirrors db.Lesson.
export interface Lesson {
  id: string
  ts: number // unix seconds of the LAST occurrence
  agentId?: string
  sessionId?: string // session of the last occurrence
  tool?: string // failing tool ("" for turn-level errors)
  sig: string // dedupe key (tool + error shape)
  text: string // the lesson itself (model-facing English)
  count: number // how many times this failure shape was seen
}
