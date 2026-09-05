package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// writeSplitSession lays down a session in the current (split) layout with
// caller-chosen timestamps — the only way to obtain a session whose last
// activity is in the past, since CreateSession/AddMessage always stamp now().
func writeSplitSession(t *testing.T, storeDir, id, header, msgs string) {
	t.Helper()
	dir := filepath.Join(storeDir, dirSessions, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, sessionHeaderFile), []byte(header), 0o644); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, sessionMsgsFile), []byte(msgs), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
}

// TestSetSessionTitleKeepsLastActivity pins the meaning of UpdatedAt: it is the
// session's LAST ACTIVITY stamp, not "last touched". Titling a session that
// finished long ago must leave it alone — the rota screen draws each bar from
// CreatedAt to UpdatedAt, so bumping it would stretch a finished session's bar
// all the way to now.
func TestSetSessionTitleKeepsLastActivity(t *testing.T) {
	ctx := context.Background()
	storeDir := filepath.Join(t.TempDir(), "store")
	writeSplitSession(t, storeDir, "SES1",
		`{"id":"SES1","agentId":"AG1","kind":"chat","state":"active","createdAt":100,"updatedAt":120}`,
		`{"id":"M1","sessionId":"SES1","role":"user","text":"bir","createdAt":110}
{"id":"M2","sessionId":"SES1","role":"assistant","text":"iki","createdAt":120}
`)

	d, err := Open(storeDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()

	if err := d.SetSessionTitle(ctx, "SES1", "generated much later"); err != nil {
		t.Fatalf("set title: %v", err)
	}

	got, err := d.GetSession(ctx, "SES1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.Title != "generated much later" {
		t.Fatalf("Title = %q, want %q", got.Title, "generated much later")
	}
	if got.UpdatedAt != 120 {
		t.Fatalf("UpdatedAt = %d, want 120 (the last message's time)", got.UpdatedAt)
	}
	// The header write must carry the preserved stamp to disk too, otherwise the
	// bar snaps back to "now" only until the next restart.
	if hdr := readSessionHeaderFile(t, storeDir, "SES1"); hdr.UpdatedAt != 120 {
		t.Fatalf("on-disk UpdatedAt = %d, want 120", hdr.UpdatedAt)
	}
}

// TestSetSessionTitleThenMessageAdvancesActivity is the other half of the
// contract: real activity after a title change still moves UpdatedAt forward.
func TestSetSessionTitleThenMessageAdvancesActivity(t *testing.T) {
	ctx := context.Background()
	storeDir := filepath.Join(t.TempDir(), "store")
	writeSplitSession(t, storeDir, "SES1",
		`{"id":"SES1","agentId":"AG1","kind":"chat","state":"active","createdAt":100,"updatedAt":120}`,
		`{"id":"M1","sessionId":"SES1","role":"user","text":"bir","createdAt":120}
`)

	d, err := Open(storeDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()

	if err := d.SetSessionTitle(ctx, "SES1", "renamed"); err != nil {
		t.Fatalf("set title: %v", err)
	}
	m, err := d.AddMessage(ctx, Message{SessionID: "SES1", Role: "user", Text: "uc"})
	if err != nil {
		t.Fatalf("add message: %v", err)
	}
	got, err := d.GetSession(ctx, "SES1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.UpdatedAt != m.CreatedAt {
		t.Fatalf("UpdatedAt = %d, want the new message's time %d", got.UpdatedAt, m.CreatedAt)
	}
}
