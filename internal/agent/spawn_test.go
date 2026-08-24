package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

func TestMain(m *testing.M) {
	if os.Getenv("TIONHARNESS_PREFLIGHT_HELPER") == "broken" {
		fmt.Fprintln(os.Stderr, `error loading C:\Users\user\.codex\config.toml: unknown field features.rmcp_client at line 4`)
		os.Exit(2)
	}
	os.Exit(m.Run())
}

// drainSpawns waits for all fire-and-forget spawn goroutines to finish so the
// t.TempDir() cleanup does not race their background writes (Windows locks the
// session file while it is being written, failing RemoveAll).
func drainSpawns(t *testing.T, rt *Runtime) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for rt.spawnActive.Load() > 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
}

// TestSpawnSession_OpensIndependentSession verifies the synchronous part of a
// spawn: a fresh "spawned"-kind session is created with the prompt recorded as
// the opening user turn, and the result carries the new session id + agent name.
// (The background turn fails — no provider key — but that is asynchronous and not
// asserted here.)
func TestSpawnSession_OpensIndependentSession(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Worker", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	res, err := rt.SpawnSession(ctx, agent.ID, "Do the thing", SpawnOptions{CreatedBy: agent.ID})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if res.SessionID == "" {
		t.Fatal("expected a session id")
	}
	if res.AgentName != "Worker" {
		t.Errorf("agent name = %q, want Worker", res.AgentName)
	}

	sess, err := rt.db.GetSession(ctx, res.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if sess.Kind != "spawned" {
		t.Errorf("session kind = %q, want spawned", sess.Kind)
	}
	if sess.SourceID == "" {
		t.Error("spawned session should carry a unique sourceID")
	}

	msgs, err := rt.db.ListMessages(ctx, res.SessionID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) == 0 || msgs[0].Role != "user" || msgs[0].Text != "Do the thing" {
		t.Fatalf("first message should be the user prompt, got %+v", msgs)
	}
	drainSpawns(t, rt)
}

// TestSpawnSession_ResolvesByName confirms a spawn target may be given by display
// name (not just id), and that two spawns produce two distinct sessions.
func TestSpawnSession_ResolvesByNameAndIsIndependent(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	if _, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Scout", Provider: "anthropic", Model: "m"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	a, err := rt.SpawnSession(ctx, "Scout", "task A", SpawnOptions{})
	if err != nil {
		t.Fatalf("spawn A: %v", err)
	}
	b, err := rt.SpawnSession(ctx, "Scout", "task B", SpawnOptions{})
	if err != nil {
		t.Fatalf("spawn B: %v", err)
	}
	if a.SessionID == b.SessionID {
		t.Fatal("each spawn must open its own independent session")
	}
	drainSpawns(t, rt)
}

// TestSpawnSession_RejectsEmptyPrompt guards the precondition.
func TestSpawnSession_RejectsEmptyPrompt(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	if _, err := rt.db.CreateAgent(ctx, db.Agent{Name: "W", Provider: "anthropic", Model: "m"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := rt.SpawnSession(ctx, "W", "   ", SpawnOptions{}); err == nil {
		t.Fatal("expected an error for an empty prompt")
	}
}

func TestSpawnSessionRefusesBrokenCLIWithoutCreatingSession(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("TIONHARNESS_PREFLIGHT_HELPER", "broken")
	rt.providers.SetInstances([]providers.Instance{{
		ID: "broken-codex", KindID: "codex-cli", Enabled: true,
		Values: map[string]string{
			providers.FieldKeyCLIPath:   os.Args[0],
			providers.FieldKeyConfigDir: home,
		},
	}})
	agent, err := rt.db.CreateAgent(ctx, db.Agent{
		Name: "Broken CLI", Provider: "codex-cli", ProviderInstanceID: "broken-codex", Model: "m",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	before, err := rt.db.ListSessions(ctx, "")
	if err != nil {
		t.Fatalf("list sessions before spawn: %v", err)
	}

	_, err = rt.SpawnSession(ctx, agent.ID, "must not launch", SpawnOptions{})
	if err == nil {
		t.Fatal("expected broken CLI spawn to be refused")
	}
	msg := err.Error()
	for _, want := range []string{"spawn refused: codex-cli CLI preflight failed", `C:\Users\user\.codex\config.toml`, "features.rmcp_client", "line 4"} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal missing %q: %s", want, msg)
		}
	}
	after, err := rt.db.ListSessions(ctx, "")
	if err != nil {
		t.Fatalf("list sessions after spawn: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("refused spawn created a session: before=%d after=%d", len(before), len(after))
	}
}

// TestSpawnConcurrencyCap verifies the spawn-storm brake: the slot counter
// refuses acquisitions past the configured concurrency cap, and a release frees
// a slot again.
func TestSpawnConcurrencyCap(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	rt.tun.SetSpawnLimits(2, 0)

	if !rt.acquireSpawnSlot() || !rt.acquireSpawnSlot() {
		t.Fatal("first two slots should be available")
	}
	if rt.acquireSpawnSlot() {
		t.Fatal("third slot must be refused (cap = 2)")
	}
	rt.releaseSpawnSlot()
	if !rt.acquireSpawnSlot() {
		t.Fatal("a slot should be available again after a release")
	}
}
