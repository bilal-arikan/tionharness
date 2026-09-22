package treepin

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newRepo creates a repository with an in-scope and an out-of-scope file.
func newRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	run(t, dir, "init", "-q")
	run(t, dir, "config", "user.email", "t@example.com")
	run(t, dir, "config", "user.name", "t")
	run(t, dir, "config", "core.autocrlf", "false")
	write(t, dir, "card/a.go", "package a\n")
	write(t, dir, "other/b.go", "package b\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", "init")
	return dir
}

func mustVerify(t *testing.T, dir string, p Pin) Result {
	t.Helper()
	r, err := Verify(dir, p)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	return r
}

// The TSK772 failure: unrelated files change (dirty edits, new files, even a new
// commit touching only them) while the card's files stay put. That must stay
// FRESH and be reported as ignored churn.
func TestOutOfScopeChurnStaysFresh(t *testing.T) {
	dir := newRepo(t)
	p, err := Capture(dir, []string{"card"})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	write(t, dir, "other/b.go", "package b // edited\n")
	write(t, dir, "other/new.go", "package b\n")
	run(t, dir, "add", "other/new.go")
	run(t, dir, "commit", "-q", "-m", "unrelated", "--", "other/new.go")

	r := mustVerify(t, dir, p)
	if r.Stale {
		t.Fatalf("out-of-scope churn made the pin stale: %+v", r)
	}
	if !r.HeadMoved || r.DirtyOutOfScope != 1 {
		t.Errorf("churn not reported: HeadMoved=%v DirtyOutOfScope=%d", r.HeadMoved, r.DirtyOutOfScope)
	}
}

func TestInScopeChangesGoStale(t *testing.T) {
	cases := map[string]func(t *testing.T, dir string){
		"edit":     func(t *testing.T, dir string) { write(t, dir, "card/a.go", "package a // edited\n") },
		"new file": func(t *testing.T, dir string) { write(t, dir, "card/c.go", "package a\n") },
		"delete": func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, "card", "a.go")); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			dir := newRepo(t)
			p, err := Capture(dir, []string{"card"})
			if err != nil {
				t.Fatalf("Capture: %v", err)
			}
			mutate(t, dir)
			r := mustVerify(t, dir, p)
			if !r.Stale {
				t.Fatalf("in-scope %s did not make the pin stale", name)
			}
			if len(r.DirtyInScope) == 0 {
				t.Errorf("stale result names no dirty in-scope file")
			}
		})
	}
}

// Committing exactly the content that was validated is not a change: the tree
// on disk is still the one the verdict describes.
func TestCommittingValidatedContentStaysFresh(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "card/a.go", "package a // wip\n")
	p, err := Capture(dir, []string{"card/a.go"})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	run(t, dir, "commit", "-q", "-am", "land it")
	if r := mustVerify(t, dir, p); r.Stale {
		t.Fatalf("committing the validated content made the pin stale: %+v", r)
	}
}

func TestCaptureRequiresScope(t *testing.T) {
	dir := newRepo(t)
	if _, err := Capture(dir, []string{" ", ""}); err == nil {
		t.Fatal("Capture accepted an empty scope")
	}
}

// Capture from a subdirectory still treats scope as repository-relative.
func TestCaptureFromSubdirectory(t *testing.T) {
	dir := newRepo(t)
	fromRoot, err := Capture(dir, []string{"card"})
	if err != nil {
		t.Fatal(err)
	}
	fromSub, err := Capture(filepath.Join(dir, "other"), []string{"card"})
	if err != nil {
		t.Fatal(err)
	}
	if fromRoot.Digest != fromSub.Digest || fromSub.Files != 1 {
		t.Fatalf("subdirectory capture differs: root=%+v sub=%+v", fromRoot, fromSub)
	}
}

func TestTokenRoundTrip(t *testing.T) {
	p := Pin{Head: "abc123", Scope: []string{"card/a b.go", "internal/x"}, Digest: "ff00", Files: 2}
	got, err := Parse(p.String())
	if err != nil {
		t.Fatalf("Parse(%q): %v", p.String(), err)
	}
	if got.Head != p.Head || got.Digest != p.Digest || got.Files != p.Files || len(got.Scope) != 2 || got.Scope[0] != "card/a b.go" {
		t.Fatalf("round trip = %+v, want %+v", got, p)
	}
	for _, bad := range []string{"", "treepin0 head=x", "treepin1 head=x files=1 digest=y", "treepin1 head= files=1 digest=y scope=a"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) accepted a malformed token", bad)
		}
	}
}
