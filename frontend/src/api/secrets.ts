// Per-workspace secret vault: list (masked), set (write-only value), reveal a
// single value on demand, and delete.
import type { Secret } from '@/types'
import { req } from './client'

export const secretApi = {
  listSecrets: () => req<Secret[]>('/api/secrets'),
  setSecret: (name: string, value: string, description = '') =>
    req<Secret>('/api/secrets', {
      method: 'POST',
      body: JSON.stringify({ name, value, description }),
    }),
  revealSecret: (name: string) =>
    req<{ name: string; value: string }>(
      `/api/secrets/${encodeURIComponent(name)}/reveal`,
    ),
  deleteSecret: (name: string) =>
    req<{ result: string }>(`/api/secrets/${encodeURIComponent(name)}`, {
      method: 'DELETE',
    }),
}
