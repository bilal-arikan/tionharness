import type { Session } from '@/types'

export function sessionMatchesQuery(session: Pick<Session, 'id' | 'title'>, query: string) {
  const normalizedQuery = query.trim().toLowerCase()
  if (!normalizedQuery) return true

  return (
    (session.title || 'Yeni sohbet').toLowerCase().includes(normalizedQuery) ||
    session.id.toLowerCase().includes(normalizedQuery)
  )
}
