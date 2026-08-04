package conversation

import (
	"strings"
	"testing"
)

// HandoffEnv.Todos was declared and rendered from the start but NEVER populated
// by the caller, so every handoff shipped without the one piece of state a
// resuming agent most needs: what is already done and what is still open. The
// producer side is fixed in agent.handoffEnv; these pin the rendering contract
// that made the gap invisible.

func TestHandoffEnvRendersTheChecklist(t *testing.T) {
	env := HandoffEnv{
		WorkingDir: "/w",
		Todos:      "1. [x] şemayı çıkar\n2. [~] testleri güncelle\n3. [ ] dokümanı yaz",
	}
	out := env.rendered()

	if !strings.Contains(out, "- Active todos:") {
		t.Fatalf("checklist section missing:\n%s", out)
	}
	// Every item must survive: a handoff that lists only the open work invites a
	// fresh agent to redo the finished items.
	for _, want := range []string{"[x] şemayı çıkar", "[~] testleri güncelle", "[ ] dokümanı yaz"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestHandoffEnvOmitsAnAbsentChecklist(t *testing.T) {
	// A session that never used todo_write must not grow an empty "Active todos"
	// heading — an empty section reads like a checklist that was wiped.
	out := HandoffEnv{WorkingDir: "/w", GitBranch: "main"}.rendered()
	if strings.Contains(out, "Active todos") {
		t.Errorf("empty checklist rendered a heading anyway:\n%s", out)
	}
	if !strings.Contains(out, "- Working directory: /w") {
		t.Errorf("the rest of the snapshot went missing:\n%s", out)
	}
}
