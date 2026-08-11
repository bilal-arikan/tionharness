package tools

import (
	"path/filepath"
	"testing"
)

// A confined sandbox must keep every path inside Root while no longer punishing an
// agent for spelling an in-Root path absolutely: confinement guards against escape,
// not against the absolute form.
func TestConfinedSandbox_HonoursInRootAbsolute(t *testing.T) {
	root := t.TempDir()
	sb := NewConfinedSandbox(root)

	inRootAbs := filepath.Join(root, "backend", "internal", "x.go")
	got, err := sb.Resolve(inRootAbs)
	if err != nil {
		t.Fatalf("in-root absolute path rejected: %v", err)
	}
	if want := filepath.Clean(inRootAbs); got != want {
		t.Errorf("resolved = %q, want %q", got, want)
	}

	// A relative path still resolves against Root.
	rel, err := sb.Resolve(filepath.Join("backend", "x.go"))
	if err != nil {
		t.Fatalf("relative path rejected: %v", err)
	}
	if want := filepath.Join(root, "backend", "x.go"); rel != want {
		t.Errorf("relative resolved = %q, want %q", rel, want)
	}
}

// The escape boundary is unchanged: an absolute path OUTSIDE Root, and a ".."
// traversal, are both rejected.
func TestConfinedSandbox_RejectsEscapes(t *testing.T) {
	root := t.TempDir()
	sb := NewConfinedSandbox(root)

	outsideAbs := filepath.Join(filepath.Dir(root), "sibling", "secret.go")
	if _, err := sb.Resolve(outsideAbs); err == nil {
		t.Errorf("out-of-root absolute path %q was allowed", outsideAbs)
	}
	if _, err := sb.Resolve(filepath.Join("..", "escape.go")); err == nil {
		t.Error("\"..\" traversal was allowed")
	}
}
