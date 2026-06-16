// A user-supplied file (or pasted long text) sent with a chat message. Mirrors
// db.Attachment on the backend. Created by POST /api/uploads and echoed back on
// the persisted user message.

export type AttachmentKind =
  | 'image'
  | 'text'
  | 'code'
  | 'pdf'
  | 'office'
  | 'archive'
  | 'audio'
  | 'video'
  | 'file'

export interface Attachment {
  id: string
  name: string
  mime: string
  kind: AttachmentKind
  size: number
  // Path under the workspace sandbox root (e.g. "uploads/<sid>/<id>-<name>").
  relPath?: string
  // Inlined content for text/code attachments (capped server-side).
  textContent?: string
}
