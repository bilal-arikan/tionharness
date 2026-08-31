package api

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
)

// TestToolCallErrorWithNilRunDoesNotPanic pins finding #6: toolCallError guards
// `run != nil` when it builds the known-name set, then used to call
// run.toolAllowedFor() and run.providerOf() unconditionally. Both take the run's
// mutex, so a nil run panicked inside the MCP request goroutine instead of
// answering. The nil case means "no run-scoped policy and no recorded provider",
// which must still produce an ordinary diagnostic string.
func TestToolCallErrorWithNilRunDoesNotPanic(t *testing.T) {
	tun := agent.NewTunables()
	b := &interactionBackend{runs: newChatRuns(), tun: tun}
	tok := b.runs.interactionToken("ws1", "s1", "a1")

	defs := interactionToolSpecs(tun, false)
	if len(defs) == 0 {
		t.Fatal("no interaction tool specs to probe")
	}
	for _, def := range defs {
		// Known name, no run: either allowed ("") or the activation notice, never a panic.
		msg := b.toolCallError(tok, def.Name, nil)
		if msg != "" && !strings.Contains(msg, "not activated") {
			t.Fatalf("tool %s with nil run returned %q, want \"\" or an activation notice", def.Name, msg)
		}
		if msg := b.toolCallError(tok, extendedNSPrefix+def.Name, nil); msg != "" && !strings.Contains(msg, "not activated") {
			t.Fatalf("namespaced tool %s with nil run returned %q", def.Name, msg)
		}
	}

	// Unknown name, no run: the ordinary "no such tool" guidance.
	if msg := b.toolCallError(tok, "definitely_not_a_tool", nil); !strings.Contains(msg, "No such tool available") {
		t.Fatalf("unknown tool with nil run returned %q", msg)
	}
}
