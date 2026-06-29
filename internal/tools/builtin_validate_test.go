package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func callJSON(t *testing.T, tool interface {
	Call(context.Context, json.RawMessage) (string, error)
}, args map[string]any) (string, error) {
	t.Helper()
	raw, _ := json.Marshal(args)
	return tool.Call(context.Background(), raw)
}

func TestMermaidValidate(t *testing.T) {
	tool := NewMermaidValidateTool()

	out, err := callJSON(t, tool, map[string]any{"code": "graph LR\n  A[Start] --> B{OK?}\n  B --> C[End]"})
	if err != nil {
		t.Fatalf("valid diagram: %v", err)
	}
	if !strings.HasPrefix(out, "VALID") {
		t.Fatalf("expected VALID, got: %s", out)
	}

	// Unknown type → invalid.
	out, _ = callJSON(t, tool, map[string]any{"code": "notADiagram foo\n A-->B"})
	if !strings.HasPrefix(out, "INVALID") {
		t.Fatalf("expected INVALID for unknown type, got: %s", out)
	}

	// Unbalanced bracket → invalid.
	out, _ = callJSON(t, tool, map[string]any{"code": "graph LR\n A[Start --> B"})
	if !strings.HasPrefix(out, "INVALID") {
		t.Fatalf("expected INVALID for unbalanced bracket, got: %s", out)
	}

	// Empty → error.
	if _, err := callJSON(t, tool, map[string]any{"code": "  "}); err == nil {
		t.Fatal("expected error for empty code")
	}
}

func TestConfigValidate(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "settings.json")
	_ = os.WriteFile(good, []byte(`{"defaultProvider":"claude-cli","theme":"dark"}`), 0o644)
	bad := filepath.Join(dir, "broken.json")
	_ = os.WriteFile(bad, []byte(`{"a":1,`), 0o644)
	missingKey := filepath.Join(dir, "settings2.json")
	// recognised settings shape but missing required defaultProvider
	_ = os.WriteFile(missingKey, []byte(`{"theme":"dark"}`), 0o644)
	_ = os.Rename(missingKey, filepath.Join(dir, "settings.json")) // keep name match
	tool := NewConfigValidateTool(NewSandbox(dir))

	out, err := callJSON(t, tool, map[string]any{"path": "settings.json"})
	if err != nil {
		t.Fatalf("good config: %v", err)
	}
	// settings.json now is the missing-key variant → VALID JSON but warns.
	if !strings.HasPrefix(out, "VALID") {
		t.Fatalf("expected VALID JSON, got: %s", out)
	}

	out, _ = callJSON(t, tool, map[string]any{"path": "broken.json"})
	if !strings.HasPrefix(out, "INVALID") {
		t.Fatalf("expected INVALID for malformed JSON, got: %s", out)
	}

	out, _ = callJSON(t, tool, map[string]any{"path": "nope.json"})
	if !strings.Contains(out, "not found") {
		t.Fatalf("expected not-found, got: %s", out)
	}
}

// fakeSkillValidator implements SkillValidator for the tool test.
type fakeSkillValidator struct{ res SkillValidation }

func (f fakeSkillValidator) ValidateSkill(string) SkillValidation { return f.res }

func TestSkillValidateTool(t *testing.T) {
	valid := NewSkillValidateTool(fakeSkillValidator{res: SkillValidation{Found: true, Tier: "workspace", Valid: true}})
	out, err := callJSON(t, valid, map[string]any{"skillSlug": "my-skill"})
	if err != nil || !strings.HasPrefix(out, "VALID") {
		t.Fatalf("expected VALID, got %q err=%v", out, err)
	}

	bad := NewSkillValidateTool(fakeSkillValidator{res: SkillValidation{Found: true, Tier: "workspace", Valid: false, Errors: []string{"frontmatter is missing required field: name"}}})
	out, _ = callJSON(t, bad, map[string]any{"skillSlug": "my-skill"})
	if !strings.HasPrefix(out, "INVALID") || !strings.Contains(out, "name") {
		t.Fatalf("expected INVALID with error, got: %s", out)
	}

	// Empty slug → error.
	if _, err := callJSON(t, valid, map[string]any{"skillSlug": ""}); err == nil {
		t.Fatal("expected error for empty slug")
	}
}
