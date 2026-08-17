package api

import (
	"os"
	"path/filepath"
)

// purgeQueuedAttachments deletes the on-disk files of queued turns that were
// cancelled before they ever ran. Every upload writes its own
// artifacts/<sessionId>/<uuid>-<name> file and that path is referenced by exactly
// one queued turn, so once the turn is dropped from the queue nothing can reach
// the file again: no user message was persisted, and the attachment→artifact
// capture only runs when a turn actually executes. Without this the files pile up
// in the workspace sandbox forever.
//
// Best-effort: a file already gone is fine, and a RelPath that resolves outside
// the sandbox root is skipped rather than removed.
func (s *Server) purgeQueuedAttachments(wsID string, items []inboxItem) {
	if len(items) == 0 {
		return
	}
	wsp := s.workspaceByID(wsID)
	if wsp == nil {
		return
	}
	root := wsp.SandboxRoot()
	for _, it := range items {
		for _, a := range it.Req.Attachments {
			if a.RelPath == "" {
				continue // pasted text that was never written to disk
			}
			abs := filepath.Join(root, filepath.FromSlash(a.RelPath))
			if !withinDir(root, abs) {
				continue
			}
			if err := os.Remove(abs); err != nil && !os.IsNotExist(err) && s.logger != nil {
				s.logger.Warn("queue cancel: attachment cleanup failed", "path", a.RelPath, "err", err)
			}
		}
	}
}
