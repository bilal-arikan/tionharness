package tools

import (
	"strings"
	"testing"
)

// TestShellRunInBackgroundSchemaGating: run_in_background is advertised ONLY when
// a background-shell manager is wired. Without one the runtime rejects it at call
// time, so it must not appear in the schema (nor the description) — don't offer
// what you'll refuse.
func TestShellRunInBackgroundSchemaGating(t *testing.T) {
	sb := NewSandbox(t.TempDir())

	noBg := NewShellTool(sb).Def()
	if strings.Contains(string(noBg.InputSchema), "run_in_background") {
		t.Fatalf("no-manager schema must NOT offer run_in_background:\n%s", noBg.InputSchema)
	}
	if strings.Contains(noBg.Description, "run_in_background") {
		t.Fatal("no-manager description must not mention run_in_background")
	}

	withBg := NewShellTool(sb).WithManager(NewShellManager()).Def()
	if !strings.Contains(string(withBg.InputSchema), "run_in_background") {
		t.Fatalf("manager-wired schema MUST offer run_in_background:\n%s", withBg.InputSchema)
	}
	if !strings.Contains(withBg.Description, "run_in_background") {
		t.Fatal("manager-wired description should mention run_in_background")
	}

	// PowerShell sibling follows the same gating.
	psNoBg := NewPowerShellTool(sb).Def()
	if strings.Contains(string(psNoBg.InputSchema), "run_in_background") {
		t.Fatalf("PowerShell no-manager schema must NOT offer run_in_background:\n%s", psNoBg.InputSchema)
	}
}
