import type { Session } from '@/types'
import { i18next } from '@/i18n'

export function sessionMatchesQuery(session: Pick<Session, 'id' | 'title'>, query: string) {
  const normalizedQuery = query.trim().toLowerCase()
  if (!normalizedQuery) return true

  return (
    (session.title || i18next.t('sidebar.newConversation', { ns: 'sessions' }))
      .toLowerCase()
      .includes(normalizedQuery) || session.id.toLowerCase().includes(normalizedQuery)
  )
}
