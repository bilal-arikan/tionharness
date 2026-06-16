// File upload endpoint: stores a user attachment (or pasted text blob) under the
// workspace and returns its descriptor, which is then sent with the chat turn.
import type { Attachment } from '../types'
import { getActiveWorkspace, errorFromResponse } from './client'

export const uploadsApi = {
  // uploadFile posts one file (multipart) for the given session and returns the
  // stored Attachment. Pasted text is uploaded as a text/plain File by the caller.
  uploadFile: async (sessionId: string, file: File): Promise<Attachment> => {
    const fd = new FormData()
    fd.append('sessionId', sessionId)
    fd.append('file', file)
    // IMPORTANT: do NOT set Content-Type — the browser must add the multipart
    // boundary itself, so we only forward the workspace scope header.
    const ws = getActiveWorkspace()
    const headers: Record<string, string> = {}
    if (ws) headers['X-Workspace-Id'] = ws
    const res = await fetch('/api/uploads', {
      method: 'POST',
      headers,
      body: fd,
    })
    if (!res.ok) throw new Error(await errorFromResponse(res))
    return (await res.json()) as Attachment
  },
}
