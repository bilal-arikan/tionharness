package db

// Attachment is a user-supplied file (or pasted long text) sent alongside a chat
// message. Binary files are stored on disk under the workspace uploads directory
// and referenced by RelPath (relative to the workspace sandbox root, so an
// agent's read_file tool can open them). Short text/pasted attachments also carry
// their content inline in TextContent so the model sees them without a tool call.
type Attachment struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Mime is the detected content type (e.g. "image/png", "text/plain").
	Mime string `json:"mime"`
	// Kind is a coarse category used for icon/rendering decisions:
	// "image" | "text" | "pdf" | "office" | "code" | "archive" | "audio" |
	// "video" | "file". Pasted clipboard text uses "text".
	Kind string `json:"kind"`
	// Size is the byte length of the stored file.
	Size int64 `json:"size"`
	// RelPath is the path under the workspace sandbox root (e.g.
	// "uploads/<sessionId>/<id>-<name>"). Empty for pure pasted text that was not
	// written to disk. Agents read attachments through this path.
	RelPath string `json:"relPath,omitempty"`
	// TextContent inlines the content of text/pasted attachments (capped) so it
	// can be folded directly into the provider message. Empty for binary kinds.
	TextContent string `json:"textContent,omitempty"`
}
