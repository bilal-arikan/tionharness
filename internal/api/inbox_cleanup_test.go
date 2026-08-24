package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// writeUpload creates an attachment file under the workspace sandbox root the way
// POST /api/uploads does, and returns its Attachment descriptor.
func writeUpload(t *testing.T, root, rel string) db.Attachment {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(abs, []byte("payload"), 0o644); err != nil {
		t.Fatalf("write upload: %v", err)
	}
	return db.Attachment{ID: "a1", Name: filepath.Base(rel), Kind: "file", RelPath: rel}
}

// seedQueue parks items in a session's queue WITHOUT going through
// enqueueMessage: that path kicks the serial worker, which would pop the head into
// the in-flight slot and race these tests. Here the queue content is the fixture.
func seedQueue(s *Server, wsID, sessionID string, items ...inboxItem) {
	s.inbox.lock()
	defer s.inbox.unlock()
	s.inbox.sessions[scopeKey(wsID, sessionID)] = &sessionInbox{
		items:   items,
		seen:    make(map[string]bool),
		wsID:    wsID,
		running: true, // no worker will be started for this fixture
	}
}

// queuedItem builds one waiting turn carrying the given attachments.
func queuedItem(wsID, sessionID, clientMsgID string, atts ...db.Attachment) inboxItem {
	return inboxItem{
		ClientMsgID: clientMsgID,
		Req:         chatReq{SessionID: sessionID, Message: clientMsgID, Attachments: atts},
		WorkspaceID: wsID,
	}
}

// TestCancelQueuedRemovesAttachmentFiles: a queued turn cancelled before it ran is
// the only reference to its uploaded files (no user message was persisted), so the
// files must not survive in the workspace sandbox.
func TestCancelQueuedRemovesAttachmentFiles(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	root := wsp.SandboxRoot()
	att := writeUpload(t, root, "artifacts/SES1/ab12-notes.txt")
	seedQueue(s, wsp.ID, "SES1", queuedItem(wsp.ID, "SES1", "cmid-1", att))

	if !s.cancelQueued(wsp.ID, "SES1", "cmid-1") {
		t.Fatal("cancelQueued reported nothing removed")
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(att.RelPath))); !os.IsNotExist(err) {
		t.Fatalf("attachment survived a queue cancel: %v", err)
	}
}

// TestClearQueuedRemovesAttachmentFiles: same guarantee for "clear the whole queue".
func TestClearQueuedRemovesAttachmentFiles(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	root := wsp.SandboxRoot()
	a1 := writeUpload(t, root, "artifacts/SES1/aa-one.txt")
	a2 := writeUpload(t, root, "artifacts/SES1/bb-two.txt")

	seedQueue(s, wsp.ID, "SES1",
		queuedItem(wsp.ID, "SES1", "cmid-1", a1),
		queuedItem(wsp.ID, "SES1", "cmid-2", a2),
	)

	if n := s.clearQueued(wsp.ID, "SES1"); n != 2 {
		t.Fatalf("cleared = %d, want 2", n)
	}
	for _, a := range []db.Attachment{a1, a2} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(a.RelPath))); !os.IsNotExist(err) {
			t.Fatalf("attachment %s survived a queue clear: %v", a.RelPath, err)
		}
	}
}

// TestPurgeQueuedAttachmentsRejectsEscapingPath: a RelPath that climbs out of the
// sandbox root must be skipped, never removed.
func TestPurgeQueuedAttachmentsRejectsEscapingPath(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	outside := filepath.Join(t.TempDir(), "keep.txt")
	if err := os.WriteFile(outside, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	rel, err := filepath.Rel(wsp.SandboxRoot(), outside)
	if err != nil {
		t.Fatalf("rel: %v", err)
	}
	item := inboxItem{Req: chatReq{Attachments: []db.Attachment{{RelPath: filepath.ToSlash(rel)}}}}
	s.purgeQueuedAttachments(wsp.ID, []inboxItem{item})
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("file outside the sandbox must survive: %v", err)
	}
}
