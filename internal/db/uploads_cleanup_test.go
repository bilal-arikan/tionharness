package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestDeleteSessionRemovesArtifacts verifies that deleting a session also removes
// its artifacts (the entity JSONs and their per-session file folder under
// workspace/artifacts/<sid>/), so files do not outlive the session.
func TestDeleteSessionRemovesArtifacts(t *testing.T) {
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
	// A text artifact in the session writes a file under artifacts/<sid>/.
	art, err := d.CreateArtifact(ctx, Artifact{SessionID: sess.ID, Title: "Note", Kind: ArtifactMarkdown, Content: "body"})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	dir := d.ArtifactsDir(sess.ID)
	if _, err := os.Stat(filepath.Join(dir, art.ID+".md")); err != nil {
		t.Fatalf("artifact file missing: %v", err)
	}

	if err := d.DeleteSession(ctx, sess.ID); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if _, err := d.GetArtifact(ctx, art.ID); err == nil {
		t.Errorf("artifact entity should be removed with the session")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("artifacts dir should be removed, stat err = %v", err)
	}
}

// TestDeleteAgentRemovesSessionArtifacts verifies the agent-deletion cascade also
// clears its sessions' artifacts.
func TestDeleteAgentRemovesSessionArtifacts(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})
	art, _ := d.CreateArtifact(ctx, Artifact{SessionID: sess.ID, Title: "N", Kind: ArtifactText, Content: "x"})
	dir := d.ArtifactsDir(sess.ID)

	if err := d.DeleteAgent(ctx, agent.ID); err != nil {
		t.Fatalf("delete agent: %v", err)
	}
	if _, err := d.GetArtifact(ctx, art.ID); err == nil {
		t.Errorf("artifact should be removed with the cascaded session")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("artifacts dir should be removed, stat err = %v", err)
	}
}
