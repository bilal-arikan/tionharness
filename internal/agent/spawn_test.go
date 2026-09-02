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

// drainSpawns waits for all fire-and-forget background goroutines to finish so the
// t.TempDir() cleanup does not race their writes (Windows refuses to unlink a file
// that is still open, failing RemoveAll — POSIX allows it, which is why this only
// ever broke on Windows).
//
// spawnActive alone is NOT a sufficient barrier. A worker's last act is to notify
// its coordinator, and NotifyCoordinator starts drainCoordinator in its OWN
// goroutine, which is not counted by spawnActive: the worker goroutine then exits,
// spawnActive drops to 0, the test returns, and the still-running coordinator turn
// keeps writing to <tmp>/store/sessions/<coord>/ while RemoveAll walks it. Measured
// directly: at the moment spawnActive hit 0, drainCoordinator was live and the
// coordinator's turn slot was busy in 6 of 6 runs.
//
// So this also waits for every coordinator drain loop to stop driving and for the
// per-session turn slots to clear.
func drainSpawns(t *testing.T, rt *Runtime) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !rt.backgroundWorkPending() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	if rt.backgroundWorkPending() {
		// Not fatal: the assertions have already run, and failing here would turn a
		// slow machine into a red build. It IS worth reporting — a drain that never
		// settles is the condition that makes cleanup flaky.
		t.Logf("drainSpawns: background work still pending after 5s (spawnActive=%d queued=%d)",
			rt.spawnActive.Load(), rt.spawnQueueLen())
	}
}

// backgroundWorkPending reports whether any fire-and-forget goroutine that writes to
// the store is still in flight: a spawn/worker turn, a queued spawn, or a coordinator
// drain loop (which outlives the worker that armed it).
func (r *Runtime) backgroundWorkPending() bool {
	// Linearize the queued -> active hand-off with takeQueuedSpawn. Reading
	// spawnActive before spawnQueueLen can observe active=0 from before dispatch
	// and queued=0 from after dispatch, even though one worker is live. The queue
	// mutex covers both the slot reservation and queue removal in takeQueuedSpawn,
	// so sampling both values under it closes that impossible mixed snapshot.
	r.spawnQueue.mu.Lock()
	queued := len(r.spawnQueue.shallow) + len(r.spawnQueue.deep)
	active := r.spawnActive.Load()
	r.spawnQueue.mu.Unlock()
	if active > 0 || queued > 0 || r.trajectoryWorkPending() {
		return true
	}
	pending := false
	r.coordSlots.Range(func(key, value any) bool {
		slot, ok := value.(*coordSlot)
		if !ok {
			return true
		}
		slot.mu.Lock()
		busy := slot.driving || slot.pending || slot.workerPending
		slot.mu.Unlock()
		if !busy {
			if id, ok := key.(string); ok && r.sessionTurnBusy(id) {
				busy = true
			}
		}
		if busy {
			pending = true
			return false
		}
		return true
	})
	return pending
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

// TestSpawnSession_AttributesPromptToSpawningAgent: when an AGENT orders a spawn,
// the opening prompt is that agent speaking, so it must be stamped with the
// participant fields the frontend keys its peer bubble on (role "user" +
// authorKind "agent" + authorId). Without them the prompt renders as the human's
// own turn and the reader cannot tell an agent wrote it (TSK507).
func TestSpawnSession_AttributesPromptToSpawningAgent(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	caller, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Lead", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create caller: %v", err)
	}
	target, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Helper", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}

	res, err := rt.SpawnSession(ctx, target.ID, "Do the thing", SpawnOptions{CreatedBy: caller.ID})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	msgs, err := rt.db.ListMessages(ctx, res.SessionID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("expected the opening prompt")
	}
	first := msgs[0]
	if first.AuthorKind != db.AuthorAgent || first.AuthorID != caller.ID || first.RecipientID != target.ID {
		t.Errorf("participant fields wrong: kind=%q author=%q recipient=%q (want agent/%s/%s)",
			first.AuthorKind, first.AuthorID, first.RecipientID, caller.ID, target.ID)
	}
	drainSpawns(t, rt)
}

// TestSpawnSession_HumanSpawnStaysUnattributed is the other half of the rule: a
// spawn with no agent author (a UI/API spawn, or a provenance string like
// "automation:<id>" that no agent owns) must stay a plain bubble. Stamping an
// unresolvable authorId would render an unnamed peer message and misattribute a
// human's words to an agent.
func TestSpawnSession_HumanSpawnStaysUnattributed(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	target, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Helper", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	for name, createdBy := range map[string]string{
		"user spawn":       "",
		"automation spawn": "automation:AUT1",
	} {
		t.Run(name, func(t *testing.T) {
			res, err := rt.SpawnSession(ctx, target.ID, "Do the thing", SpawnOptions{CreatedBy: createdBy})
			if err != nil {
				t.Fatalf("spawn: %v", err)
			}
			msgs, err := rt.db.ListMessages(ctx, res.SessionID)
			if err != nil {
				t.Fatalf("list messages: %v", err)
			}
			if len(msgs) == 0 {
				t.Fatal("expected the opening prompt")
			}
			if got := msgs[0].AuthorKind; got == db.AuthorAgent {
				t.Errorf("authorKind = %q, want anything but %q for a non-agent spawn", got, db.AuthorAgent)
			}
		})
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

func TestSpawnSessionStartsWithFreshPromptEpoch(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	rt.SetPromptEpoch(true)
	ctx := context.Background()
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Fresh Worker", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	parentID := "SES_stale_parent"
	rt.EpochStaticSystem(ctx, parentID, agent, false, false, "", func() string { return "PARENT OLD" })
	rt.EpochStaticSystem(ctx, parentID, agent, false, false, "", func() string { return "PARENT NEW" })

	res, err := rt.SpawnSession(ctx, agent.ID, "fresh task", SpawnOptions{ParentSessionID: parentID})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if rt.PromptEpochStale(res.SessionID, agent.ID) {
		t.Fatal("spawned session must not inherit the parent's stale prompt epoch")
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
	rt.tun.SetSpawnLimits(2, 0, 0)

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
