export function shouldShowSessionsLoadMore({
  loading,
  hasMoreSessions,
  canLoadMore,
  query,
}: {
  loading: boolean
  hasMoreSessions: boolean
  canLoadMore: boolean
  query: string
  hasActiveChipFilters: boolean
  filteredSessionCount: number
}) {
  return !loading && hasMoreSessions && canLoadMore && query.trim().length < 2
}
