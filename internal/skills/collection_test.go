package skills

import (
	"os"
	"path/filepath"
	"testing"
)

// skillMD is a minimal valid SKILL.md body for a folder named after the slug.
func skillMD(name, desc string) []byte {
	return []byte("---\nname: " + name + "\ndescription: " + desc + "\n---\nBody for " + name + ".\n")
}

func TestDiscoverInTreeGrouping(t *testing.T) {
	files := map[string][]byte{
		"skills/ab-testing/SKILL.md":            skillMD("AB Testing", "split tests"),
		"skills/ab-testing/references/guide.md": []byte("ref"),
		"skills/ab-testing/evals/case1.json":    []byte("{}"),
		"skills/copywriting/SKILL.md":           skillMD("Copywriting", "write copy"),
		"skills/copywriting/README.md":          []byte("readme"),
		// a nested sub-skill: its resources must NOT be claimed by the parent
		"skills/copywriting/headlines/SKILL.md": skillMD("Headlines", "punchy headlines"),
		"skills/copywriting/headlines/tips.md":  []byte("tips"),
		// repo noise + unrelated top-level files (no owner)
		".github/workflows/ci.yml": []byte("ci"),
		"README.md":               []byte("top"),
		"LICENSE":                 []byte("mit"),
	}
	found := discoverInTree(files, "")
	if len(found) != 3 {
		t.Fatalf("want 3 skills, got %d: %+v", len(found), found)
	}
	byPath := map[string]discoveredSkill{}
	for _, d := range found {
		byPath[d.relPath] = d
	}
	ab, ok := byPath["skills/ab-testing"]
	if !ok {
		t.Fatal("ab-testing not discovered")
	}
	if _, has := ab.files["references/guide.md"]; !has {
		t.Errorf("ab-testing missing nested references/guide.md: %v", keysOf(ab.files))
	}
	if _, has := ab.files["evals/case1.json"]; !has {
		t.Errorf("ab-testing missing nested evals/case1.json: %v", keysOf(ab.files))
	}
	cw := byPath["skills/copywriting"]
	if _, has := cw.files["README.md"]; !has {
		t.Errorf("copywriting missing README.md: %v", keysOf(cw.files))
	}
	// The nested sub-skill's files must belong to it, NOT to copywriting.
	if _, leaked := cw.files["headlines/tips.md"]; leaked {
		t.Errorf("parent copywriting wrongly claimed nested sub-skill file: %v", keysOf(cw.files))
	}
	hl := byPath["skills/copywriting/headlines"]
	if _, has := hl.files["tips.md"]; !has {
		t.Errorf("headlines sub-skill missing its own tips.md: %v", keysOf(hl.files))
	}
}

func TestDiscoverInTreePrefixFilter(t *testing.T) {
	files := map[string][]byte{
		"skills/keep/SKILL.md":  skillMD("Keep", "k"),
		"other/drop/SKILL.md":   skillMD("Drop", "d"),
		"plugins/p/x/SKILL.md":  skillMD("X", "x"),
	}
	found := discoverInTree(files, "skills")
	if len(found) != 1 || found[0].relPath != "skills/keep" {
		t.Fatalf("prefix filter failed: %+v", found)
	}
}

func TestDiscoverInTreeRootSkill(t *testing.T) {
	// A skill at the tree root: SKILL.md at top, top-level resources belong to it.
	files := map[string][]byte{
		"SKILL.md":      skillMD("Root", "root skill"),
		"reference.md":  []byte("ref"),
		"scripts/go.sh": []byte("echo"),
	}
	found := discoverInTree(files, "")
	if len(found) != 1 {
		t.Fatalf("want 1 root skill, got %d", len(found))
	}
	d := found[0]
	if d.relPath != "" {
		t.Errorf("root skill relPath should be empty, got %q", d.relPath)
	}
	if _, has := d.files["reference.md"]; !has {
		t.Errorf("root skill missing reference.md: %v", keysOf(d.files))
	}
	if _, has := d.files["scripts/go.sh"]; !has {
		t.Errorf("root skill missing scripts/go.sh: %v", keysOf(d.files))
	}
}

func TestSafeBundledPath(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"reference.md", "reference.md", true},
		{"references/guide.md", filepath.FromSlash("references/guide.md"), true},
		{"./a/b.txt", filepath.FromSlash("a/b.txt"), true},
		{"SKILL.md", "", false},
		{"skill.md", "", false}, // case-insensitive SKILL.md
		{"../escape.txt", "", false},
		{"a/../../escape.txt", "", false},
		{"/abs/path.txt", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := safeBundledPath(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("safeBundledPath(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestImportCollectionLocalNested(t *testing.T) {
	root := t.TempDir()
	// Build a small collection on disk.
	writeFile(t, root, "skills/alpha/SKILL.md", skillMD("Alpha", "first"))
	writeFile(t, root, "skills/alpha/references/r.md", []byte("ref"))
	writeFile(t, root, "skills/beta/SKILL.md", skillMD("Beta", "second"))

	ws := t.TempDir()
	s := New("", ws)
	res, err := s.ImportCollection("local", root, nil, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Imported) != 2 {
		t.Fatalf("want 2 imported, got %d (skipped=%v)", len(res.Imported), res.Skipped)
	}
	// Nested resource must be written under the skill folder.
	if _, err := os.Stat(filepath.Join(ws, "alpha", "references", "r.md")); err != nil {
		t.Errorf("nested resource not written: %v", err)
	}
	if _, ok := s.Get("alpha"); !ok {
		t.Errorf("alpha not resolved after import")
	}
	if _, ok := s.Get("beta"); !ok {
		t.Errorf("beta not resolved after import")
	}
}

func TestImportCollectionPrefixAndSelectionAndCollision(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/alpha/SKILL.md", skillMD("Alpha", "a"))
	writeFile(t, root, "skills/beta/SKILL.md", skillMD("Beta", "b"))

	ws := t.TempDir()
	s := New("", ws)
	// Select only alpha, namespaced with a prefix.
	res, err := s.ImportCollection("local", root, []string{"skills/alpha"}, "pack", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Imported) != 1 || res.Imported[0].Slug != "pack-alpha" {
		t.Fatalf("prefix/selection failed: %+v", res.Imported)
	}
	if _, ok := s.Get("pack-beta"); ok {
		t.Errorf("beta should not have been imported")
	}
	// Re-import all without prefix → alpha/beta succeed; a second run collides.
	res2, _ := s.ImportCollection("local", root, nil, "", false)
	if len(res2.Imported) != 2 {
		t.Fatalf("want 2 imported on first unprefixed run, got %d", len(res2.Imported))
	}
	res3, _ := s.ImportCollection("local", root, nil, "", false)
	if len(res3.Imported) != 0 || len(res3.Skipped) != 2 {
		t.Errorf("collision run should skip both: imported=%d skipped=%d", len(res3.Imported), len(res3.Skipped))
	}
}

func TestNormalizeRepoRef(t *testing.T) {
	cases := map[string]string{
		"owner/repo":                          "https://github.com/owner/repo",
		"owner/repo/tree/main/skills":         "https://github.com/owner/repo/tree/main/skills",
		"https://github.com/owner/repo":       "https://github.com/owner/repo",
		"github.com/owner/repo":               "github.com/owner/repo",
	}
	for in, want := range cases {
		if got := normalizeRepoRef(in); got != want {
			t.Errorf("normalizeRepoRef(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseFrontmatterBlockScalar(t *testing.T) {
	// Folded (>) description spanning multiple indented lines, as caveman uses.
	raw := "---\nname: caveman\ndescription: >\n  Ultra-compressed mode. Cuts tokens ~75%\n  while keeping accuracy.\nversion: 1.0\n---\nbody"
	fm, body := parseFrontmatter(raw)
	if got := fm.scalar("name"); got != "caveman" {
		t.Errorf("name = %q", got)
	}
	want := "Ultra-compressed mode. Cuts tokens ~75% while keeping accuracy."
	if got := fm.scalar("description"); got != want {
		t.Errorf("folded description = %q, want %q", got, want)
	}
	// The key AFTER the block scalar must still parse (continuation consumed correctly).
	if got := fm.scalar("version"); got != "1.0" {
		t.Errorf("version after block scalar = %q, want 1.0", got)
	}
	if body != "body" {
		t.Errorf("body = %q", body)
	}

	// Literal (|) preserves line breaks.
	raw2 := "---\ndescription: |\n  line one\n  line two\n---\nx"
	fm2, _ := parseFrontmatter(raw2)
	if got := fm2.scalar("description"); got != "line one\nline two" {
		t.Errorf("literal description = %q", got)
	}
}

// --- helpers ---

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func writeFile(t *testing.T, root, rel string, data []byte) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
