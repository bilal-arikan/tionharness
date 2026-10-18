import type { TurnStep } from '@/types'

export function isExecutionNotice(step: TurnStep): boolean {
  return ['cli_interrupted', 'cli_resume', 'mcp_startup', 'delivery_check'].includes(
    step.operation ?? '',
  )
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
