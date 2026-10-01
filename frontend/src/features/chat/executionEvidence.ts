import type { TurnStep } from '@/types'

export function notificationTraceInvalid(stepsJSON?: string): boolean {
  if (!stepsJSON) return false
  try {
    const steps: unknown = JSON.parse(stepsJSON)
    return !Array.isArray(steps) || steps.some((step) => !step || typeof step !== 'object')
  } catch {
    return true
  }
}

export function isExecutionNotice(step: TurnStep): boolean {
  return ['cli_interrupted', 'cli_resume', 'mcp_startup', 'delivery_check'].includes(
    step.operation ?? '',
  )
}

export function notificationExecutionSteps(stepsJSON?: string): TurnStep[] {
  try {
    const steps: unknown = JSON.parse(stepsJSON || '[]')
    return Array.isArray(steps)
      ? steps.filter((step): step is TurnStep =>
          Boolean(step && typeof step === 'object' && isExecutionNotice(step)),
        )
      : []
  } catch {
    return []
  }
}

export function deliveryFiles(output?: string): { path: string; status: string }[] {
  try {
    const files: unknown = JSON.parse(output || '[]')
    return Array.isArray(files)
      ? files
          .filter(
            (file) => file && typeof file.path === 'string' && typeof file.status === 'string',
          )
          .slice(0, 16)
      : []
  } catch {
    return []
  }
}
