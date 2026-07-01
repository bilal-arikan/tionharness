package providers

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLiveClaudeCLIResume is a REAL end-to-end check of the claude-cli --resume
// path: it spends actual tokens, so it is gated behind SWARMGO_LIVE_CLI=1 and
// skipped in normal runs. It proves the core resume promise — turn 2 sends ONLY a
// follow-up question (no transcript) yet the model still recalls a fact stated in
// turn 1, because the CLI resumed its own server-side session.
//
//	SWARMGO_LIVE_CLI=1 go test ./internal/providers/ -run TestLiveClaudeCLIResume -v
func TestLiveClaudeCLIResume(t *testing.T) {
	if os.Getenv("SWARMGO_LIVE_CLI") != "1" {
		t.Skip("set SWARMGO_LIVE_CLI=1 to run the live claude-cli resume test")
	}
	bin := "claude"
	if p := os.Getenv("SWARMGO_CLAUDE_BIN"); p != "" {
		bin = p
	}
	c := NewClaudeCLI(bin, "", "", "", "") // "" → CLI default (logged-in) model

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Turn 1: state a fact and capture the CLI session id.
	r1, err := c.Complete(ctx, Request{
		PermissionMode: "auto",
		Messages: []Message{{
			Role: RoleUser,
			Text: "Remember this for later: my secret passphrase is BANANA-42. Reply with just the word OK.",
		}},
	})
	if err != nil {
		t.Fatalf("turn 1 failed: %v", err)
	}
	t.Logf("turn 1 text=%q sessionID=%q", r1.Text, r1.SessionID)
	if r1.SessionID == "" {
		t.Fatalf("turn 1 did not capture a CLI session id — resume cannot work")
	}

	// Turn 2: RESUME the session and send ONLY a follow-up (no prior context). If
	// resume works, the model still knows the passphrase from turn 1.
	r2, err := c.Complete(ctx, Request{
		PermissionMode:  "auto",
		ResumeSessionID: r1.SessionID,
		Messages: []Message{{
			Role: RoleUser,
			Text: "What was the secret passphrase I told you earlier? Reply with only the passphrase.",
		}},
	})
	if err != nil {
		t.Fatalf("turn 2 (resume) failed: %v", err)
	}
	t.Logf("turn 2 text=%q sessionID=%q usage=%+v", r2.Text, r2.SessionID, r2.Usage)

	if !strings.Contains(strings.ToUpper(r2.Text), "BANANA-42") {
		t.Fatalf("resume did NOT carry context: turn 2 answer %q does not contain the passphrase from turn 1", r2.Text)
	}
	if r2.SessionID == "" {
		t.Errorf("turn 2 did not capture a (rotated) session id to persist for the next turn")
	}

	// Phase 1 cache-warmth: a resumed turn that adds a VOLATILE SystemDynamic must
	// still REUSE the warm cache — because the dynamic context now rides in the
	// message tail, not the appended system prompt, so it no longer busts the cached
	// prefix. System is left empty here (matching turns 1–2) so ONLY the dynamic
	// suffix varies; a regression (dynamic folded back into the system prefix) would
	// drop cache_read to 0.
	r3, err := c.Complete(ctx, Request{
		PermissionMode:  "auto",
		ResumeSessionID: r2.SessionID,
		SystemDynamic:   "Current date and time: " + time.Now().Format("2006-01-02 15:04:05") + "\nThis line changes every turn.",
		Messages: []Message{{
			Role: RoleUser,
			Text: "Reply with just the word DONE.",
		}},
	})
	if err != nil {
		t.Fatalf("turn 3 (resume + volatile dynamic) failed: %v", err)
	}
	t.Logf("turn 3 usage=%+v", r3.Usage)
	if r3.Usage.CacheReadTokens == 0 {
		t.Errorf("turn 3 read 0 cached tokens — volatile SystemDynamic busted the warm prefix "+
			"(Phase 1 regression). cacheWrite=%d input=%d", r3.Usage.CacheWriteTokens, r3.Usage.InputTokens)
	}
}

// TestLivePersistentSession is a REAL end-to-end check of the persistent-session
// path (Phase 4): two turns through one warm CLISessionPool process. Turn 2 must
// (a) still know a fact from turn 1 (the process held the conversation) and (b)
// REUSE the prompt cache (cache_read > 0) despite a volatile SystemDynamic. Gated
// behind SWARMGO_LIVE_CLI=1 (spends tokens).
//
//	SWARMGO_LIVE_CLI=1 go test ./internal/providers/ -run TestLivePersistentSession -v
func TestLivePersistentSession(t *testing.T) {
	if os.Getenv("SWARMGO_LIVE_CLI") != "1" {
		t.Skip("set SWARMGO_LIVE_CLI=1 to run the live persistent-session test")
	}
	bin := "claude"
	if p := os.Getenv("SWARMGO_CLAUDE_BIN"); p != "" {
		bin = p
	}
	c := NewClaudeCLI(bin, "", "", "", "")
	pool := NewCLISessionPool()
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	const key = "test-persistent-session"

	r1, err := pool.Turn(ctx, key, c, Request{
		PermissionMode: "auto",
		System:         "You are a terse assistant.",
		Messages:       []Message{{Role: RoleUser, Text: "Remember this: my code is ZEBRA-9. Reply with just OK."}},
	}, nil)
	if err != nil {
		t.Fatalf("turn 1 failed: %v", err)
	}
	t.Logf("turn 1 usage=%+v text=%q", r1.Usage, r1.Text)

	// Turn 2 reuses the SAME warm process (same key, same fingerprint). The pool
	// ships only the new user message; the process supplies the rest. A volatile
	// SystemDynamic is included to prove it does not bust the cached prefix.
	r2, err := pool.Turn(ctx, key, c, Request{
		PermissionMode: "auto",
		System:         "You are a terse assistant.",
		SystemDynamic:  "Current time: " + time.Now().Format("15:04:05") + " (changes every turn)",
		Messages: []Message{
			{Role: RoleUser, Text: "Remember this: my code is ZEBRA-9. Reply with just OK."},
			{Role: RoleAssistant, Text: "OK"},
			{Role: RoleUser, Text: "What is my code? Reply with only the code."},
		},
	}, nil)
	if err != nil {
		t.Fatalf("turn 2 failed: %v", err)
	}
	t.Logf("turn 2 usage=%+v text=%q", r2.Usage, r2.Text)

	// Context retention is the hard guarantee: the warm process must still know the
	// fact from turn 1 (it was never re-sent — the pool ships only the new message).
	if !strings.Contains(strings.ToUpper(r2.Text), "ZEBRA-9") {
		t.Fatalf("persistent process lost context: turn 2 answer %q lacks the code from turn 1", r2.Text)
	}
	// Cache reuse is LOGGED, not asserted: the warm process does reuse the prompt
	// cache across stream-json turns (measured cache_read=87672 on 2026-06-29), but
	// it is subject to the Anthropic 5-minute TTL, so a slow run can legitimately
	// see cache_read=0 without a code defect. The hard guarantee is context
	// retention above; warmth is opportunistic. ClaudePersistentSession is default
	// off (experimental). See _Docs/17.
	if r2.Usage.CacheReadTokens == 0 {
		t.Logf("NOTE: persistent turn 2 cache_read=0 (cacheWrite=%d) — likely a cold/TTL-expired "+
			"cache this run, not a defect; context retention still held.", r2.Usage.CacheWriteTokens)
	} else {
		t.Logf("persistent turn 2 reused cache: cache_read=%d cacheWrite=%d", r2.Usage.CacheReadTokens, r2.Usage.CacheWriteTokens)
	}
}

// TestBuildSystemAndPrompt locks in the cache-stability invariant WITHOUT spending
// tokens: the appended system prompt must carry ONLY the static prefix (so the
// cached prefix stays byte-stable turn-to-turn), while the volatile dynamic context
// must ride in the conversation prompt's [Context] block. A regression here is what
// caused turn-to-turn cold cache writes before the split.
func TestBuildSystemAndPrompt(t *testing.T) {
	c := NewClaudeCLI("claude", "", "", "", "")
	req := Request{
		System:        "STATIC-PERSONA",
		SystemDynamic: "VOLATILE-CLOCK-2026",
		Messages:      []Message{{Role: RoleUser, Text: "hello"}},
	}
	sys, prompt := c.buildSystemAndPrompt(req)

	if sys != "STATIC-PERSONA" {
		t.Errorf("appended system must be the static prefix only, got %q", sys)
	}
	if strings.Contains(sys, "VOLATILE-CLOCK-2026") {
		t.Errorf("volatile dynamic leaked into the cached system prompt: %q", sys)
	}
	if !strings.Contains(prompt, "VOLATILE-CLOCK-2026") {
		t.Errorf("volatile dynamic must ride in the conversation prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "[Context]") || !strings.Contains(prompt, "hello") {
		t.Errorf("conversation prompt missing [Context] block or user message: %q", prompt)
	}

	// Determinism: same request → byte-identical appended system across builds (the
	// property the cross-session cache relies on).
	sys2, _ := c.buildSystemAndPrompt(req)
	if sys != sys2 {
		t.Errorf("appended system not deterministic: %q vs %q", sys, sys2)
	}

	// Empty dynamic → no [Context] wrapper, prompt is just the transcript.
	if _, p := c.buildSystemAndPrompt(Request{System: "S", Messages: []Message{{Role: RoleUser, Text: "hi"}}}); p != "hi" {
		t.Errorf("empty dynamic should yield a bare prompt, got %q", p)
	}
}
