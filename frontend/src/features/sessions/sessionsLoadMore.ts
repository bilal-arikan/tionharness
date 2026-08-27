export function shouldShowSessionsLoadMore({
  loading,
  hasMoreSessions,
  canLoadMore,
  query,
  hasActiveChipFilters,
  filteredSessionCount,
}: {
  loading: boolean
  hasMoreSessions: boolean
  canLoadMore: boolean
  query: string
  hasActiveChipFilters: boolean
  filteredSessionCount: number
}) {
  return (
    !loading &&
    hasMoreSessions &&
    canLoadMore &&
    query.trim().length < 2 &&
    (!hasActiveChipFilters || filteredSessionCount > 0)
  )
}
