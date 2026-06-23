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
	c := NewClaudeCLI(bin, "") // "" → CLI default (logged-in) model

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
	t.Logf("turn 2 text=%q sessionID=%q", r2.Text, r2.SessionID)

	if !strings.Contains(strings.ToUpper(r2.Text), "BANANA-42") {
		t.Fatalf("resume did NOT carry context: turn 2 answer %q does not contain the passphrase from turn 1", r2.Text)
	}
	if r2.SessionID == "" {
		t.Errorf("turn 2 did not capture a (rotated) session id to persist for the next turn")
	}
}
