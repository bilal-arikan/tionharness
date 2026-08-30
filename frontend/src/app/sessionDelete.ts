export async function deleteSessionAndRefresh(
  sessionId: string,
  deleteSession: (id: string) => Promise<unknown>,
  refreshSessions: () => void,
): Promise<void> {
  await deleteSession(sessionId)
  refreshSessions()
}
