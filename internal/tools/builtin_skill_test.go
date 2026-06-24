package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type fakeSkillLib struct{ allowed []string }

func (f fakeSkillLib) Body(string) (string, error)  { return "do the thing", nil }
func (f fakeSkillLib) AllowedTools(string) []string { return f.allowed }

// TestUseSkillGrantsToolsSK3 covers SK-3: loading a skill auto-grants its declared
// tool patterns to the session grant set, argument-scoped where given.
func TestUseSkillGrantsToolsSK3(t *testing.T) {
	g := NewPermissionGrants()
	ctx := WithGrants(context.Background(), g)
	tool := NewUseSkillTool(fakeSkillLib{allowed: []string{"Bash(git *)", "Read"}})

	out, err := tool.Call(ctx, json.RawMessage(`{"slug":"demo"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !g.Matches("Bash", "git status") {
		t.Errorf("Bash(git *) should be granted for git args")
	}
	if g.Matches("Bash", "rm -rf /") {
		t.Errorf("Bash(git *) must NOT grant arbitrary commands")
	}
	if !g.Granted("Read") {
		t.Errorf("whole-tool Read grant missing")
	}
	if !strings.Contains(out, "auto-allowed") {
		t.Errorf("expected transparency note in output: %q", out)
	}
}

// No grant store on the context → no panic, no grants, normal body returned.
func TestUseSkillNoGrantsContext(t *testing.T) {
	tool := NewUseSkillTool(fakeSkillLib{allowed: []string{"Read"}})
	out, err := tool.Call(context.Background(), json.RawMessage(`{"slug":"demo"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "do the thing") {
		t.Errorf("body missing: %q", out)
	}
	if strings.Contains(out, "auto-allowed") {
		t.Errorf("should not claim grants without a grant store: %q", out)
	}
}
