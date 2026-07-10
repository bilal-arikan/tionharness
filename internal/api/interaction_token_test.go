package api

import "testing"

// TestInteractionTokenStablePerSessionAgent locks the Doc 52 §3-D fix: the
// Interaction MCP Bearer token is STABLE per (session,agent) — reused across turns so
// the persistent claude-cli mcp-config stays byte-identical — and byToken resolves the
// reused token to whichever run is currently in flight (via bindActive).
func TestInteractionTokenStablePerSessionAgent(t *testing.T) {
	runs := newChatRuns()

	// Same (session,agent) → same token across calls (the warm-reuse invariant).
	t1 := runs.interactionToken("s1", "a1")
	t1b := runs.interactionToken("s1", "a1")
	if t1 == "" || t1 != t1b {
		t.Fatalf("token must be stable per (session,agent): %q vs %q", t1, t1b)
	}

	// Different agent (same session) → different token (per-agent isolation).
	if t2 := runs.interactionToken("s1", "a2"); t2 == t1 {
		t.Fatalf("different agent must get a different token, both got %q", t1)
	}

	// A live run bound to the stable token resolves via byToken.
	run := runs.register("run-1", "s1", "", func() {})
	runs.bindActive(t1, run)
	if got := runs.byToken(t1); got != run {
		t.Fatalf("byToken(stable) = %v, want the bound run", got)
	}

	// After the turn ends the binding is cleared (no resolving to a dead turn)...
	runs.unregister("run-1")
	if got := runs.byToken(t1); got != nil {
		t.Fatalf("byToken after unregister = %v, want nil", got)
	}
	// ...but the secret survives so the NEXT turn reuses the same token.
	if t1c := runs.interactionToken("s1", "a1"); t1c != t1 {
		t.Fatalf("secret must survive unregister: %q != %q", t1c, t1)
	}
}

// TestByTokenPerRunFallback verifies the legacy per-run token path still resolves
// (autonomous turns / callers that did not bind a stable token).
func TestByTokenPerRunFallback(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("run-x", "s9", "", func() {})
	if got := runs.byToken(run.token); got != run {
		t.Fatalf("byToken(run.token) fallback = %v, want the run", got)
	}
	if got := runs.byToken(""); got != nil {
		t.Fatalf("empty token must resolve to nil, got %v", got)
	}
}
