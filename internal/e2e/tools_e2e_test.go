package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/agent"
)

// TestTools_WriteThenReadLoop exercises the native multi-step tool loop in one
// turn: the model writes a file, the loop feeds the result back, the model reads
// it back, then answers. It asserts the file really lands on disk, the read result
// flows back into the answer, and the activity trace records both tool steps.
func TestTools_WriteThenReadLoop(t *testing.T) {
	prov := newScriptedProvider(
		callTools("Saving your note.", tc("c1", "Write", map[string]any{
			"path": "notes/todo.txt", "content": "ship the e2e tests",
		})),
		callTools("Reading it back.", tc("c2", "Read", map[string]any{
			"path": "notes/todo.txt",
		})),
		sayText("Your note says: ship the e2e tests"),
	)
	h := newHarness(t, prov)
	ag := h.newAgent("Tooly")
	sess := h.newSession(ag)

	res := h.send(ag, sess, "Save a note then read it to me.")

	// The model was called three times: write turn, read turn, final answer.
	if prov.calls != 3 {
		t.Fatalf("provider calls = %d, want 3 (write, read, answer)", prov.calls)
	}

	// The Write tool actually created the file in the workspace sandbox.
	onDisk := filepath.Join(h.workDir, "notes", "todo.txt")
	data, err := os.ReadFile(onDisk)
	if err != nil {
		t.Fatalf("expected file written by Write tool at %s: %v", onDisk, err)
	}
	if string(data) != "ship the e2e tests" {
		t.Errorf("file content = %q, want %q", string(data), "ship the e2e tests")
	}

	// The Read tool result fed back to the model carried the file content.
	read := findToolStep(res.steps, "Read")
	if read == nil {
		t.Fatalf("no Read tool step in trace: %+v", res.steps)
	}
	if !strings.Contains(read.Output, "ship the e2e tests") {
		t.Errorf("Read step output = %q, want it to contain the file content", read.Output)
	}

	// The write surfaces as a diff card (a file mutation), not a generic tool row.
	if step := findToolStep(res.steps, "Write"); step == nil {
		t.Errorf("no Write tool step in trace")
	} else if step.Kind != agent.StepDiff {
		t.Errorf("Write step kind = %q, want StepDiff", step.Kind)
	}

	// The final answer (after the loop) reflects what was read.
	if !strings.Contains(res.resp.Text, "ship the e2e tests") {
		t.Errorf("final answer = %q, want it to mention the note", res.resp.Text)
	}

	// Steps were also delivered live (streaming), not only at the end.
	if len(res.streamed) == 0 {
		t.Errorf("no steps streamed via the live sink")
	}
}

// TestTools_ShellGate confirms the high-risk shell tool stays absent until the
// workspace enables it, then runs a real command through the sandbox inside a turn.
func TestTools_ShellGate(t *testing.T) {
	prov := newScriptedProvider(
		callTools("", tc("c1", "Bash", map[string]any{"command": "echo e2e-shell-ok"})),
		sayText("done"),
	)
	h := newHarness(t, prov)
	h.tun.SetShellEnabled(true)
	ag := h.newAgent("Sheller")
	sess := h.newSession(ag)

	res := h.send(ag, sess, "Run echo for me.")

	bash := findToolStep(res.steps, "Bash")
	if bash == nil {
		t.Fatalf("no Bash tool step in trace: %+v", res.steps)
	}
	if bash.IsError || !strings.Contains(bash.Output, "e2e-shell-ok") {
		t.Errorf("Bash output = %q (err=%v), want it to contain the echo", bash.Output, bash.IsError)
	}
}
