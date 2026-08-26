export interface DefaultAgentSaveOptions {
  nextId: string
  previousId: string | null
  setCurrent: (id: string | null) => void
  persist: (id: string) => Promise<unknown>
  reportError: (message: string) => void
}

export async function saveDefaultAgent({
  nextId,
  previousId,
  setCurrent,
  persist,
  reportError,
}: DefaultAgentSaveOptions): Promise<boolean> {
  setCurrent(nextId)
  try {
    await persist(nextId)
    return true
  } catch (error) {
    setCurrent(previousId)
    reportError(error instanceof Error ? error.message : String(error))
    return false
  }
}
