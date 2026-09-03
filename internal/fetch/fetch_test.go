package fetch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeRepoRef(t *testing.T) {
	cases := map[string]string{
		"owner/repo":                    "https://github.com/owner/repo",
		"owner/repo/tree/main/skills":   "https://github.com/owner/repo/tree/main/skills",
		"https://github.com/owner/repo": "https://github.com/owner/repo",
		"github.com/owner/repo":         "github.com/owner/repo",
	}
	for in, want := range cases {
		if got := NormalizeRepoRef(in); got != want {
			t.Errorf("NormalizeRepoRef(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseGitHubURL(t *testing.T) {
	owner, repo, ref, dir, err := ParseGitHubURL("https://github.com/o/r/tree/dev/skills/x")
	if err != nil || owner != "o" || repo != "r" || ref != "dev" || dir != "skills/x" {
		t.Fatalf("got %q/%q/%q/%q err=%v", owner, repo, ref, dir, err)
	}
	// blob → SKILL.md resolves to parent dir.
	_, _, _, dir2, _ := ParseGitHubURL("https://github.com/o/r/blob/main/skills/x/SKILL.md")
	if dir2 != "skills/x" {
		t.Errorf("blob dir = %q, want skills/x", dir2)
	}
	// bare repo → ref defaults to main, dir empty.
	_, _, ref3, dir3, _ := ParseGitHubURL("https://github.com/o/r")
	if ref3 != "main" || dir3 != "" {
		t.Errorf("bare repo ref/dir = %q/%q", ref3, dir3)
	}
	// non-github rejected.
	if _, _, _, _, e := ParseGitHubURL("https://gitlab.com/o/r"); e == nil {
		t.Errorf("expected non-github rejection")
	}
}

func TestGroupByMarkerAndFindFiles(t *testing.T) {
	tree := Tree{
		"agents/a.md":           []byte("A"),
		"agents/b.md":           []byte("B"),
		"skills/x/SKILL.md":     []byte("X"),
		"skills/x/ref/guide.md": []byte("g"),
		"skills/x/sub/SKILL.md": []byte("SUB"), // nested marker keeps its own files
		"skills/x/sub/note.md":  []byte("n"),
		"README.md":             []byte("top"),
	}
	groups := GroupByMarker(tree, "skills", "SKILL.md")
	if len(groups) != 2 {
		t.Fatalf("want 2 skill groups, got %d", len(groups))
	}
	byPath := map[string]Group{}
	for _, g := range groups {
		byPath[g.RelPath] = g
	}
	if _, ok := byPath["skills/x"].Files["ref/guide.md"]; !ok {
		t.Errorf("skills/x missing ref/guide.md: %v", byPath["skills/x"].Files)
	}
	if _, leaked := byPath["skills/x"].Files["sub/note.md"]; leaked {
		t.Errorf("parent wrongly claimed nested marker's file")
	}
	if _, ok := byPath["skills/x/sub"].Files["note.md"]; !ok {
		t.Errorf("nested marker missing its own note.md")
	}

	md := FindFiles(tree, "agents", func(n string) bool { return strings.HasSuffix(n, ".md") })
	if len(md) != 2 || md[0] != "agents/a.md" {
		t.Errorf("FindFiles agents = %v", md)
	}
}

func TestTreeFromLocal(t *testing.T) {
	root := t.TempDir()
	must := func(rel, content string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must("a/SKILL.md", "x")
	must("a/ref.md", "y")
	must(".git/config", "junk") // must be skipped

	tree, prefix, _, err := TreeFrom("local", root)
	if err != nil {
		t.Fatal(err)
	}
	if prefix != "" {
		t.Errorf("local prefix should be empty, got %q", prefix)
	}
	if _, ok := tree["a/SKILL.md"]; !ok {
		t.Errorf("a/SKILL.md missing")
	}
	if _, ok := tree[".git/config"]; ok {
		t.Errorf(".git should be skipped")
	}
}
