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

// TestDeleteAgentKeepsSessionsAndArtifacts pins the soft-delete contract: an
// agent is marked deleted, but its conversations — and everything hanging off
// them — survive, because history must stay readable after its author is gone.
func TestDeleteAgentKeepsSessionsAndArtifacts(t *testing.T) {
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
	if _, err := d.GetSession(ctx, sess.ID); err != nil {
		t.Fatalf("session must outlive its agent: %v", err)
	}
	if _, err := d.GetArtifact(ctx, art.ID); err != nil {
		t.Errorf("artifact must outlive its agent: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("artifacts dir must survive, stat err = %v", err)
	}

	// The agent row is kept so history can still render its name/avatar…
	got, err := d.GetAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("deleted agent must remain resolvable: %v", err)
	}
	if !got.Deleted || got.DeletedAt == 0 {
		t.Fatalf("agent not marked deleted: %+v", got)
	}
	if got.Name != "A" {
		t.Fatalf("deleted agent lost its identity: %q", got.Name)
	}
	// …but it must be gone from every roster/picker.
	list, _ := d.ListAgents(ctx)
	for _, a := range list {
		if a.ID == agent.ID {
			t.Fatal("deleted agent still listed in ListAgents")
		}
	}

	// It survives a reopen as deleted (the flag is persisted, not in-memory only).
	_ = d.Close()
	d2, err := Open(d.Root())
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer d2.Close()
	again, err := d2.GetAgent(ctx, agent.ID)
	if err != nil || !again.Deleted {
		t.Fatalf("deleted flag lost across reopen: %+v err=%v", again, err)
	}
}
