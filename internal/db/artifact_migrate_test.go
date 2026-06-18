package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestMigrateUnifiedLayout verifies that a pre-unification chat attachment (file
// under uploads/<sid>/ referenced by a message) is moved into the per-session
// artifacts folder, the message relPath is updated, and a backing "chat" artifact
// is created — all on Open, idempotently.
func TestMigrateUnifiedLayout(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	storeDir := filepath.Join(root, "store")
	wsDir := filepath.Join(root, "workspace")

	// Seed a session with a message carrying a legacy uploads/ attachment.
	d, err := Open(storeDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ag, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: ag.ID, Title: "T"})

	// Place a legacy upload file and a message referencing it.
	oldRel := "uploads/" + sess.ID + "/ab-shot.png"
	oldAbs := filepath.Join(wsDir, filepath.FromSlash(oldRel))
	if err := os.MkdirAll(filepath.Dir(oldAbs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldAbs, []byte("PNGDATA"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := d.AddMessage(ctx, Message{
		SessionID: sess.ID, Role: "user", Text: "see this",
		Attachments: []Attachment{{ID: "ab", Name: "shot.png", Kind: "image", RelPath: oldRel}},
	}); err != nil {
		t.Fatalf("add message: %v", err)
	}

	// Reopen → migration runs.
	d2, err := Open(storeDir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}

	// 1) File moved into artifacts/<sid>/.
	newAbs := filepath.Join(wsDir, "artifacts", sess.ID, "ab-shot.png")
	if _, err := os.Stat(newAbs); err != nil {
		t.Errorf("file not moved to per-session folder: %v", err)
	}
	if _, err := os.Stat(oldAbs); !os.IsNotExist(err) {
		t.Errorf("old upload file should be gone, stat err = %v", err)
	}

	// 2) Message relPath updated.
	msgs, _ := d2.ListMessages(ctx, sess.ID)
	if len(msgs) != 1 || len(msgs[0].Attachments) != 1 {
		t.Fatalf("unexpected messages: %+v", msgs)
	}
	if got := msgs[0].Attachments[0].RelPath; got != "artifacts/"+sess.ID+"/ab-shot.png" {
		t.Errorf("relPath not migrated: %q", got)
	}

	// 3) A backing chat artifact exists for the attachment.
	arts, _ := d2.ListArtifacts(ctx, sess.ID)
	found := false
	for _, a := range arts {
		if a.Origin == "chat" && a.SourcePath == "artifacts/"+sess.ID+"/ab-shot.png" && a.Kind == ArtifactImage {
			found = true
		}
	}
	if !found {
		t.Errorf("no backing chat artifact created; got %+v", arts)
	}
}
