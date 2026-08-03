package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// writeLegacySession lays down a pre-split session directory: one session.jsonl
// with the header on line 1 and the messages after it.
func writeLegacySession(t *testing.T, storeDir, id string, body string) {
	t.Helper()
	dir := filepath.Join(storeDir, dirSessions, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, legacySessionFile), []byte(body), 0o644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
}

// TestLegacySessionMigratesToSplitLayout covers the upgrade path for a store
// written before the header/transcript split: opening it must read the combined
// file, convert it to session.json + messages.jsonl, and drop the old file —
// without losing the header fields or a single message.
func TestLegacySessionMigratesToSplitLayout(t *testing.T) {
	ctx := context.Background()
	storeDir := filepath.Join(t.TempDir(), "store")
	writeLegacySession(t, storeDir, "SES1", `{"id":"SES1","agentId":"AG1","title":"Eski","kind":"chat","state":"active","createdAt":100,"updatedAt":100}
{"id":"M1","sessionId":"SES1","role":"user","text":"bir","createdAt":110}
{"id":"M2","sessionId":"SES1","role":"assistant","text":"iki","createdAt":120}
`)

	d, err := Open(storeDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sess, err := d.GetSession(ctx, "SES1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if sess.Title != "Eski" || sess.AgentID != "AG1" {
		t.Fatalf("header lost: %+v", sess)
	}
	if sess.MessageCount != 2 {
		t.Fatalf("MessageCount = %d, want 2", sess.MessageCount)
	}
	// UpdatedAt is reconciled from the newest message, not the stored header.
	if sess.UpdatedAt != 120 {
		t.Fatalf("UpdatedAt = %d, want 120", sess.UpdatedAt)
	}
	msgs, _ := d.ListMessages(ctx, "SES1")
	if len(msgs) != 2 || msgs[0].Text != "bir" || msgs[1].Text != "iki" {
		t.Fatalf("messages lost: %+v", msgs)
	}
	_ = d.Close()

	dir := filepath.Join(storeDir, dirSessions, "SES1")
	if _, err := os.Stat(filepath.Join(dir, sessionHeaderFile)); err != nil {
		t.Fatalf("header file not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, sessionMsgsFile)); err != nil {
		t.Fatalf("transcript file not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, legacySessionFile)); !os.IsNotExist(err) {
		t.Fatalf("legacy file should be gone, stat err = %v", err)
	}

	// Second open goes down the split path and must see the same data.
	d2, err := Open(storeDir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer d2.Close()
	msgs2, _ := d2.ListMessages(ctx, "SES1")
	if len(msgs2) != 2 {
		t.Fatalf("after migration reopen: %d messages, want 2", len(msgs2))
	}
}

// TestLegacyMigrationIsRedoneWhenHeaderMissing pins the crash-safety rule: the
// header file is the marker the loader keys on, so a migration interrupted after
// the transcript was written but before the header must be redone from the
// legacy file rather than silently reading a half-migrated directory.
func TestLegacyMigrationIsRedoneWhenHeaderMissing(t *testing.T) {
	ctx := context.Background()
	storeDir := filepath.Join(t.TempDir(), "store")
	writeLegacySession(t, storeDir, "SES1", `{"id":"SES1","title":"T","kind":"chat","state":"active","createdAt":1,"updatedAt":1}
{"id":"M1","sessionId":"SES1","role":"user","text":"bir","createdAt":2}
`)
	// Simulate the interrupted run: a stale/partial transcript exists, no header.
	dir := filepath.Join(storeDir, dirSessions, "SES1")
	if err := os.WriteFile(filepath.Join(dir, sessionMsgsFile), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("seed partial transcript: %v", err)
	}

	d, err := Open(storeDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	msgs, _ := d.ListMessages(ctx, "SES1")
	if len(msgs) != 1 || msgs[0].Text != "bir" {
		t.Fatalf("legacy file should win while the header is absent, got: %+v", msgs)
	}
}

// TestSessionDirEdgeCasesDoNotBreakBoot covers the malformed/partial session
// directories a crash or a hand-edit can leave behind. None may fail the whole
// store's Open — a single unusable directory is skipped, not fatal.
func TestSessionDirEdgeCasesDoNotBreakBoot(t *testing.T) {
	ctx := context.Background()
	storeDir := filepath.Join(t.TempDir(), "store")

	// (a) legacy file with a header but zero messages
	writeLegacySession(t, storeDir, "SES1", `{"id":"SES1","title":"bos","kind":"chat","state":"active","createdAt":1,"updatedAt":1}
`)
	// (b) completely empty legacy file
	writeLegacySession(t, storeDir, "SES2", "")
	// (c) a directory with neither layout present
	if err := os.MkdirAll(filepath.Join(storeDir, dirSessions, "SES3"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// (d) migration interrupted after the header was written: a stale legacy file
	// sits next to a complete split layout. The header is the marker, so the split
	// files win and the leftover is inert.
	dir4 := filepath.Join(storeDir, dirSessions, "SES4")
	if err := os.MkdirAll(dir4, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeLegacySession(t, storeDir, "SES4", `{"id":"SES4","title":"eski","kind":"chat","state":"active","createdAt":1,"updatedAt":1}
{"id":"OLD","sessionId":"SES4","role":"user","text":"eski mesaj","createdAt":2}
`)
	if err := os.WriteFile(filepath.Join(dir4, sessionHeaderFile),
		[]byte(`{"id":"SES4","title":"yeni","kind":"chat","state":"active","createdAt":1,"updatedAt":1}`+"\n"), 0o644); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir4, sessionMsgsFile),
		[]byte(`{"id":"NEW","sessionId":"SES4","role":"user","text":"yeni mesaj","createdAt":2}`+"\n"), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	d, err := Open(storeDir)
	if err != nil {
		t.Fatalf("open must tolerate partial session dirs, got: %v", err)
	}
	defer d.Close()

	s1, err := d.GetSession(ctx, "SES1")
	if err != nil {
		t.Fatalf("header-only session lost: %v", err)
	}
	if s1.MessageCount != 0 {
		t.Fatalf("SES1 MessageCount = %d, want 0", s1.MessageCount)
	}
	if _, err := d.GetSession(ctx, "SES2"); err == nil {
		t.Fatal("empty legacy file should yield no session")
	}
	if _, err := d.GetSession(ctx, "SES3"); err == nil {
		t.Fatal("empty directory should yield no session")
	}
	s4, err := d.GetSession(ctx, "SES4")
	if err != nil {
		t.Fatalf("split layout lost: %v", err)
	}
	if s4.Title != "yeni" {
		t.Fatalf("stale legacy file won over session.json: title = %q", s4.Title)
	}
	msgs, _ := d.ListMessages(ctx, "SES4")
	if len(msgs) != 1 || msgs[0].Text != "yeni mesaj" {
		t.Fatalf("stale legacy transcript won: %+v", msgs)
	}
}

// TestHeaderEditLeavesTranscriptFileUntouched is the whole point of the split: a
// metadata-only mutation must not rewrite the transcript.
func TestHeaderEditLeavesTranscriptFileUntouched(t *testing.T) {
	ctx := context.Background()
	storeDir := filepath.Join(t.TempDir(), "store")
	d, err := Open(storeDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "ilk"})
	for i := 0; i < 5; i++ {
		if _, err := d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "user", Text: "m"}); err != nil {
			t.Fatalf("add msg: %v", err)
		}
	}

	msgPath := filepath.Join(storeDir, dirSessions, sess.ID, sessionMsgsFile)
	before, err := os.ReadFile(msgPath)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	beforeStat, _ := os.Stat(msgPath)

	if err := d.SetSessionTitle(ctx, sess.ID, "yeni baslik"); err != nil {
		t.Fatalf("set title: %v", err)
	}

	after, err := os.ReadFile(msgPath)
	if err != nil {
		t.Fatalf("read transcript after: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("title change rewrote the transcript file")
	}
	afterStat, _ := os.Stat(msgPath)
	if !beforeStat.ModTime().Equal(afterStat.ModTime()) {
		t.Fatal("title change touched the transcript file's mtime")
	}
	got, _ := d.GetSession(ctx, sess.ID)
	if got.Title != "yeni baslik" {
		t.Fatalf("title not persisted: %q", got.Title)
	}
}
