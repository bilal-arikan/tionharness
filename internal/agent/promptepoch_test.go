package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// epochAgent is the fixed agent identity used across the epoch tests.
var epochAgent = db.Agent{ID: "AGT_epoch", Model: "claude-sonnet-5", Name: "Epoch"}

// TestEpochFreezesStaticSystem: the first turn freezes the live prefix; later
// turns keep serving the SAME bytes even when the live builder drifts, and the
// drift is reported as stale (also via PromptEpochStale) without being adopted.
func TestEpochFreezesStaticSystem(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	rt.SetPromptEpoch(true)
	ctx := context.Background()
	sid := "SES_epoch_freeze"

	got, stale := rt.EpochStaticSystem(ctx, sid, epochAgent, false, false, "", func() string { return "PREFIX v1" })
	if got != "PREFIX v1" || stale {
		t.Fatalf("first turn must freeze and serve the live prefix, got %q stale=%v", got, stale)
	}

	// Live drift: builder output changed → frozen bytes still serve, stale=true.
	got, stale = rt.EpochStaticSystem(ctx, sid, epochAgent, false, false, "", func() string { return "PREFIX v2" })
	if got != "PREFIX v1" {
		t.Errorf("frozen snapshot must keep serving v1, got %q", got)
	}
	if !stale {
		t.Error("live drift must be reported stale")
	}
	if !rt.PromptEpochStale(sid, epochAgent.ID) {
		t.Error("PromptEpochStale must report the held-back drift")
	}

	// Drift reverted → no longer stale.
	if _, stale = rt.EpochStaticSystem(ctx, sid, epochAgent, false, false, "", func() string { return "PREFIX v1" }); stale {
		t.Error("reverted drift must clear the stale flag")
	}
}

// TestEpochAdoptTriggers: every adopt trigger rebuilds the snapshot from live
// state — model change, workdir change, participants flip, forced (compaction).
func TestEpochAdoptTriggers(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	rt.SetPromptEpoch(true)
	ctx := context.Background()

	cases := []struct {
		name  string
		serve func(sid string, build func() string) string
	}{
		{"model-changed", func(sid string, build func() string) string {
			changed := epochAgent
			changed.Model = "claude-opus-4-8"
			got, _ := rt.EpochStaticSystem(ctx, sid, changed, false, false, "", build)
			return got
		}},
		{"workdir-changed", func(sid string, build func() string) string {
			got, _ := rt.EpochStaticSystem(ctx, sid, epochAgent, false, false, `C:\other`, build)
			return got
		}},
		{"participants-changed", func(sid string, build func() string) string {
			got, _ := rt.EpochStaticSystem(ctx, sid, epochAgent, true, false, "", build)
			return got
		}},
		{"compaction", func(sid string, build func() string) string {
			got, _ := rt.EpochStaticSystem(ctx, sid, epochAgent, false, true, "", build)
			return got
		}},
	}
	for _, tc := range cases {
		sid := "SES_adopt_" + tc.name
		if got, _ := rt.EpochStaticSystem(ctx, sid, epochAgent, false, false, "", func() string { return "OLD" }); got != "OLD" {
			t.Fatalf("%s: seed freeze failed: %q", tc.name, got)
		}
		if got := tc.serve(sid, func() string { return "NEW" }); got != "NEW" {
			t.Errorf("%s: trigger must adopt the live prefix, got %q", tc.name, got)
		}
	}
}

// TestEpochAdoptTTLCold: an idle gap past the cache TTL adopts pending changes
// for free (nothing cached is left to protect).
func TestEpochAdoptTTLCold(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	rt.SetPromptEpoch(true)
	ctx := context.Background()
	sid := "SES_ttl"

	if got, _ := rt.EpochStaticSystem(ctx, sid, epochAgent, false, false, "", func() string { return "OLD" }); got != "OLD" {
		t.Fatalf("seed freeze failed: %q", got)
	}
	// Age the snapshot past the TTL (package-internal access on purpose).
	rt.epochMu.Lock()
	rt.epochCache[sid][epochAgent.ID].LastUsedAt = time.Now().Add(-promptEpochAdoptAfter - time.Minute).UnixMilli()
	rt.epochMu.Unlock()

	if got, stale := rt.EpochStaticSystem(ctx, sid, epochAgent, false, false, "", func() string { return "NEW" }); got != "NEW" || stale {
		t.Errorf("TTL-cold turn must adopt the live prefix, got %q stale=%v", got, stale)
	}
}

// TestEpochDisabledPassThrough: with the workspace toggle off, every turn is
// live composition — no snapshot, no stale flag.
func TestEpochDisabledPassThrough(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	rt.SetPromptEpoch(false)
	ctx := context.Background()
	sid := "SES_disabled"

	if got, stale := rt.EpochStaticSystem(ctx, sid, epochAgent, false, false, "", func() string { return "A" }); got != "A" || stale {
		t.Fatalf("disabled: want live A, got %q stale=%v", got, stale)
	}
	if got, stale := rt.EpochStaticSystem(ctx, sid, epochAgent, false, false, "", func() string { return "B" }); got != "B" || stale {
		t.Errorf("disabled: want live B (no freezing), got %q stale=%v", got, stale)
	}
	if defs, stale := rt.EpochToolDefs(ctx, sid, epochAgent, func() []providers.ToolDef { return nil }); defs != nil || stale {
		t.Errorf("disabled: EpochToolDefs must fail open to live defs (nil), got %v stale=%v", defs, stale)
	}
}

// TestEpochSidecarSurvivesRestart: a second runtime over the SAME store serves
// the first runtime's frozen bytes (the provider-side cache survives a restart,
// so the snapshot must too).
func TestEpochSidecarSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	rt1, _ := newTestRuntime(t, dir)
	rt1.SetPromptEpoch(true)
	ctx := context.Background()
	sid := "SES_restart"

	if got, _ := rt1.EpochStaticSystem(ctx, sid, epochAgent, false, false, "", func() string { return "FROZEN" }); got != "FROZEN" {
		t.Fatalf("seed freeze failed: %q", got)
	}

	// "Restart": a fresh runtime over the same db files.
	rt2 := NewRuntime(rt1.db, providers.NewRegistry(), NewTunables(), dir, filepath.Dir(dir), nil, nil, "", "", nil, rt1.logger)
	rt2.SetPromptEpoch(true)
	if got, stale := rt2.EpochStaticSystem(ctx, sid, epochAgent, false, false, "", func() string { return "LIVE-AFTER-RESTART" }); got != "FROZEN" || !stale {
		t.Errorf("restart must serve the sidecar snapshot (stale live drift), got %q stale=%v", got, stale)
	}
}

// TestEpochToolDefsFreezeAndStale: tool defs freeze on first tool turn; passive
// catalog drift keeps serving the frozen defs and reports stale.
func TestEpochToolDefsFreezeAndStale(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	rt.SetPromptEpoch(true)
	ctx := context.Background()
	sid := "SES_tools"

	defA := providers.ToolDef{Name: "alpha", Description: "a", InputSchema: json.RawMessage(`{"type":"object"}`)}
	defB := providers.ToolDef{Name: "beta", Description: "b", InputSchema: json.RawMessage(`{"type":"object"}`)}

	// No entry yet → fail open to live (nil signals "use live defs").
	if defs, _ := rt.EpochToolDefs(ctx, sid, epochAgent, func() []providers.ToolDef { return []providers.ToolDef{defA} }); defs != nil {
		t.Fatalf("no entry: must fail open (nil), got %v", defs)
	}

	// The system side establishes the entry; the tool side then freezes.
	rt.EpochStaticSystem(ctx, sid, epochAgent, false, false, "", func() string { return "SYS" })
	defs, stale := rt.EpochToolDefs(ctx, sid, epochAgent, func() []providers.ToolDef { return []providers.ToolDef{defA} })
	if len(defs) != 1 || defs[0].Name != "alpha" || stale {
		t.Fatalf("first tool turn must freeze live defs, got %v stale=%v", defs, stale)
	}

	// Passive drift (a new tool appeared) → frozen defs serve, stale reported.
	defs, stale = rt.EpochToolDefs(ctx, sid, epochAgent, func() []providers.ToolDef { return []providers.ToolDef{defA, defB} })
	if len(defs) != 1 || defs[0].Name != "alpha" {
		t.Errorf("frozen defs must keep serving, got %v", defs)
	}
	if !stale {
		t.Error("tool catalog drift must be reported stale")
	}
}

// TestMergeFrozenToolDefs: frozen bytes/order stay put; the agent's own in-turn
// activations take their live form and unseen activated defs append at the end.
func TestMergeFrozenToolDefs(t *testing.T) {
	frozen := []providers.ToolDef{
		{Name: "alpha", Description: "frozen-a", DeferLoading: true},
		{Name: "beta", Description: "frozen-b"},
	}
	live := []providers.ToolDef{
		{Name: "alpha", Description: "live-a", DeferLoading: false}, // activated → live form
		{Name: "beta", Description: "live-b"},                       // NOT activated → frozen form
		{Name: "gamma", Description: "live-g"},                      // activated, unseen → appended
	}

	// No activations → the frozen slice verbatim.
	if got := mergeFrozenToolDefs(frozen, live, nil); len(got) != 2 || got[0].Description != "frozen-a" || got[1].Description != "frozen-b" {
		t.Fatalf("no activations must return frozen defs verbatim, got %v", got)
	}

	got := mergeFrozenToolDefs(frozen, live, map[string]bool{"alpha": true, "gamma": true})
	if len(got) != 3 {
		t.Fatalf("want 3 defs (2 frozen slots + 1 appended activation), got %v", got)
	}
	if got[0].Name != "alpha" || got[0].Description != "live-a" || got[0].DeferLoading {
		t.Errorf("activated def must take its live form in place, got %+v", got[0])
	}
	if got[1].Name != "beta" || got[1].Description != "frozen-b" {
		t.Errorf("non-activated def must keep frozen bytes, got %+v", got[1])
	}
	if got[2].Name != "gamma" || got[2].Description != "live-g" {
		t.Errorf("unseen activated def must append last, got %+v", got[2])
	}
}

// TestEpochRefresh: the explicit adopt (/refresh-context, update_session
// refresh_context) drops the snapshot so the next turn recomposes live.
func TestEpochRefresh(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	rt.SetPromptEpoch(true)
	ctx := context.Background()
	sid := "SES_refresh"

	rt.EpochStaticSystem(ctx, sid, epochAgent, false, false, "", func() string { return "OLD" })
	rt.RefreshPromptEpoch(ctx, sid)
	if got, stale := rt.EpochStaticSystem(ctx, sid, epochAgent, false, false, "", func() string { return "NEW" }); got != "NEW" || stale {
		t.Errorf("refresh must drop the snapshot (next turn live), got %q stale=%v", got, stale)
	}
	// The sidecar must be gone too, not just the memory cache.
	if _, ok, _ := rt.db.ReadPromptEpoch(sid); ok {
		// A re-freeze just happened above, so the sidecar exists again — clear and
		// verify RefreshPromptEpoch removes it.
		rt.RefreshPromptEpoch(ctx, sid)
		if _, ok2, _ := rt.db.ReadPromptEpoch(sid); ok2 {
			t.Error("RefreshPromptEpoch must remove the sidecar file")
		}
	}
}

// TestEpochPerAgentIsolation: two agents in one session freeze independent
// snapshots (multi-agent sessions key entries per agent).
func TestEpochPerAgentIsolation(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	rt.SetPromptEpoch(true)
	ctx := context.Background()
	sid := "SES_two_agents"
	other := db.Agent{ID: "AGT_other", Model: "claude-sonnet-5"}

	rt.EpochStaticSystem(ctx, sid, epochAgent, false, false, "", func() string { return "ADA" })
	rt.EpochStaticSystem(ctx, sid, other, false, false, "", func() string { return "KAI" })

	if got, _ := rt.EpochStaticSystem(ctx, sid, epochAgent, false, false, "", func() string { return "DRIFT" }); got != "ADA" {
		t.Errorf("agent 1 snapshot lost: %q", got)
	}
	if got, _ := rt.EpochStaticSystem(ctx, sid, other, false, false, "", func() string { return "DRIFT" }); got != "KAI" {
		t.Errorf("agent 2 snapshot lost: %q", got)
	}
}
