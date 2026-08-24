// Pure helpers over TurnStep[]. Split out so TurnSteps.tsx exports only
// components (fast refresh).
import type { TurnStep } from '@/types'

export function stepTruncated(s: TurnStep): boolean {
  return (
    !!s.outputTruncated ||
    !!s.textTruncated ||
    !!s.patchTruncated ||
    !!s.inputTruncated ||
    !!s.subSteps?.some(stepTruncated)
  )
}

// parseSteps safely decodes the JSON `steps` string persisted on a message.
export function parseSteps(json?: string): TurnStep[] {
  if (!json || json === '[]') return []
  try {
    const v = JSON.parse(json)
    return Array.isArray(v) ? (v as TurnStep[]) : []
  } catch {
    return []
  }
}
