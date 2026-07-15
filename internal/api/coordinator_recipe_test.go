package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/skills"
)

// writeRecipe drops a SKILL.md under <dir>/<slug>/ for the resolver test.
func writeRecipe(t *testing.T, dir, slug, content string) {
	t.Helper()
	sd := filepath.Join(dir, slug)
	if err := os.MkdirAll(sd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sd, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestResolveCoordinatorRecipe covers the validation gate: empty clears, unknown
// slug / non-workflow / bad pattern are hard errors, and a valid recipe returns
// its max_turns override.
func TestResolveCoordinatorRecipe(t *testing.T) {
	dir := t.TempDir()
	writeRecipe(t, dir, "wf-ok", "---\nname: OK\nkind: coordinator-workflow\npattern: fanout\nmax_turns: 12\n---\nbody")
	writeRecipe(t, dir, "plain", "---\nname: Plain\ndescription: normal skill\n---\nbody")
	writeRecipe(t, dir, "wf-badpat", "---\nname: Bad\nkind: coordinator-workflow\npattern: nope\n---\nbody")
	store := skills.New("", dir)

	if mt, err := ResolveCoordinatorRecipe(store, ""); err != nil || mt != 0 {
		t.Errorf("empty slug: got (%d,%v), want (0,nil)", mt, err)
	}
	if mt, err := ResolveCoordinatorRecipe(store, "wf-ok"); err != nil || mt != 12 {
		t.Errorf("valid recipe: got (%d,%v), want (12,nil)", mt, err)
	}
	if _, err := ResolveCoordinatorRecipe(store, "missing"); err == nil {
		t.Error("unknown slug should error")
	}
	if _, err := ResolveCoordinatorRecipe(store, "plain"); err == nil {
		t.Error("non-workflow skill should error")
	}
	if _, err := ResolveCoordinatorRecipe(store, "wf-badpat"); err == nil {
		t.Error("unknown pattern should error")
	}
}
