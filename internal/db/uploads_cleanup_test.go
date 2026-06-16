package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestDeleteSessionRemovesUploads verifies that deleting a session also removes
// its attachment-upload folder (workspace/uploads/<sid>), a sibling of the store
// root, so uploaded files do not outlive the session.
func TestDeleteSessionRemovesUploads(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	storeDir := filepath.Join(root, "store")

	d, err := Open(storeDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, err := d.CreateAgent(ctx, Agent{Name: "Tester", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T1"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Simulate an uploaded attachment on disk under workspace/uploads/<sid>.
	up := d.sessionUploadsDir(sess.ID)
	if err := os.MkdirAll(up, 0o755); err != nil {
		t.Fatalf("mkdir uploads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(up, "ab-note.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write upload: %v", err)
	}

	if err := d.DeleteSession(ctx, sess.ID); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if _, err := os.Stat(up); !os.IsNotExist(err) {
		t.Fatalf("uploads dir should be removed, stat err = %v", err)
	}
}

// TestDeleteAgentRemovesSessionUploads verifies the agent-deletion path (which
// cascades to its sessions) also clears their uploads.
func TestDeleteAgentRemovesSessionUploads(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})
	up := d.sessionUploadsDir(sess.ID)
	if err := os.MkdirAll(up, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := d.DeleteAgent(ctx, agent.ID); err != nil {
		t.Fatalf("delete agent: %v", err)
	}
	if _, err := os.Stat(up); !os.IsNotExist(err) {
		t.Fatalf("uploads dir should be removed, stat err = %v", err)
	}
}
