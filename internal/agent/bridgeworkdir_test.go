package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestBridgeFilesystemUsesSessionWorkdirAndFreshness(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	dir := t.TempDir()
	path := filepath.Join(dir, "document.md")
	if err := os.WriteFile(path, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	session, err := rt.db.CreateSession(ctx, db.Session{AgentID: "agent", WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	_, call := rt.BridgeTools(WithSessionID(ctx, session.ID), db.Agent{ID: "agent"})
	if _, err := call(ctx, "Read", json.RawMessage(`{"path":"document.md"}`)); err != nil {
		t.Fatalf("session Read: %v", err)
	}
	patch, _ := json.Marshal(map[string]string{"patch": "--- a/document.md\n+++ b/document.md\n@@\n-original\n+revised\n"})
	if _, err := call(ctx, "apply_patch", patch); err != nil {
		t.Fatalf("session patch: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "revised\n" {
		t.Fatalf("wrong destination: %q", got)
	}
	if err := os.WriteFile(path, []byte("external change\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	patch, _ = json.Marshal(map[string]string{"patch": "--- a/document.md\n+++ b/document.md\n@@\n-revised\n+overwrite\n"})
	if _, err := call(ctx, "apply_patch", patch); err == nil {
		t.Fatal("freshness guard did not reject concurrent change")
	}
}

func TestBridgeAutonomousFilesystemRemainsConfined(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	tun.SetWorkdirGuards(true, false)
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	session, err := rt.db.CreateSession(context.Background(), db.Session{AgentID: "agent", WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	_, call := rt.BridgeTools(WithSessionID(context.Background(), session.ID), db.Agent{ID: "agent"}, true)
	input, _ := json.Marshal(map[string]string{"path": outside})
	if _, err := call(context.Background(), "Read", input); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("unconfined autonomous read: %v", err)
	}
}
