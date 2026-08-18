// Failure lessons (self-healing) — workspace-scoped read + prune.
import type { Lesson } from '@/types'
import { req } from './client'

export const lessonApi = {
  listLessons: () => req<Lesson[]>('/api/lessons'),
  deleteLesson: (id: string) => req<{ result: string }>(`/api/lessons/${id}`, { method: 'DELETE' }),
}
