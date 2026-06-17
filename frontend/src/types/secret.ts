// Workspace secret vault entries. The value is never sent in the list view; it
// is fetched explicitly via the reveal endpoint (Show/Copy) or read by agents
// through the secret_get tool.

export interface Secret {
  name: string
  description: string
  createdAt: number
  updatedAt: number
}
