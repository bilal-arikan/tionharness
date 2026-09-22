package changelog

import (
	"os/exec"
	"testing"
)

// newTestRepo builds a throwaway git repository in a temp dir. The shared
// working tree is never touched: every git-dependent test runs here.
func newTestRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"config", "commit.gpgsign", "false"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return dir
}

// commitTo makes an empty commit with the given message.
func commitTo(t *testing.T, dir, message string) {
	t.Helper()
	cmd := exec.Command("git", "commit", "-q", "--allow-empty", "-m", message)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit %q: %v: %s", message, err, out)
	}
}

// tagAt tags HEAD.
func tagAt(t *testing.T, dir, tag string) {
	t.Helper()
	cmd := exec.Command("git", "tag", tag)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tag %q: %v: %s", tag, err, out)
	}
}

// TestBuildRangeIsBetweenVersionTags is the core of the card: the range is
// <previous version tag>..<tag>, so a release shows ONLY its own commits.
func TestBuildRangeIsBetweenVersionTags(t *testing.T) {
	dir := newTestRepo(t)
	commitTo(t, dir, "feat: first release thing")
	tagAt(t, dir, "v1.0.0")
	commitTo(t, dir, "fix(api): second release fix")
	commitTo(t, dir, "feat(ui): second release feature")
	tagAt(t, dir, "v1.1.0")

	r, err := Build(dir, "v1.1.0")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if r.Previous != "v1.0.0" {
		t.Fatalf("previous tag is %q, want v1.0.0", r.Previous)
	}
	if r.Count != 2 {
		t.Fatalf("range holds %d commits, want 2 (the first release leaked in)", r.Count)
	}
	if r.Version != "1.1.0" {
		t.Fatalf("version is %q, want 1.1.0", r.Version)
	}
}

// TestBuildFirstReleaseCoversWholeHistory: with no predecessor the range is
// open-ended. An empty Previous is a real case, not an error.
func TestBuildFirstReleaseCoversWholeHistory(t *testing.T) {
	dir := newTestRepo(t)
	commitTo(t, dir, "feat: one")
	commitTo(t, dir, "fix: two")
	tagAt(t, dir, "v0.1.0")

	r, err := Build(dir, "v0.1.0")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if r.Previous != "" {
		t.Fatalf("a first release claims predecessor %q", r.Previous)
	}
	if r.Count != 2 {
		t.Fatalf("first release holds %d commits, want 2", r.Count)
	}
}

// TestPreviousTagIgnoresNonVersionTags: this repository carries a
// `before-rename` marker tag. Treating it as a release boundary would silently
// truncate a changelog range.
func TestPreviousTagIgnoresNonVersionTags(t *testing.T) {
	dir := newTestRepo(t)
	commitTo(t, dir, "feat: early")
	tagAt(t, dir, "v1.0.0")
	commitTo(t, dir, "chore: marker point")
	tagAt(t, dir, "before-rename")
	commitTo(t, dir, "feat: later")
	tagAt(t, dir, "v1.1.0")

	prev, err := PreviousTag(dir, "v1.1.0")
	if err != nil {
		t.Fatalf("PreviousTag: %v", err)
	}
	if prev != "v1.0.0" {
		t.Fatalf("previous tag is %q, want v1.0.0 (a marker tag was used as a boundary)", prev)
	}
}

// TestCommitsPreservesMultilineBodies: the body carries the BREAKING CHANGE
// footer, so a record separator that cannot survive newlines would lose it.
func TestCommitsPreservesMultilineBodies(t *testing.T) {
	dir := newTestRepo(t)
	commitTo(t, dir, "feat: something\n\nA longer explanation.\n\nBREAKING CHANGE: the old flag is gone")
	tagAt(t, dir, "v1.0.0")

	commits, err := Commits(dir, "", "v1.0.0")
	if err != nil {
		t.Fatalf("Commits: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("got %d commits, want 1", len(commits))
	}
	if !commits[0].Breaking {
		t.Fatal("the BREAKING CHANGE footer did not survive the git log format")
	}
	if commits[0].Subject != "something" {
		t.Fatalf("subject is %q", commits[0].Subject)
	}
}

// TestCommitsExcludesMerges: a merge carries no change of its own and its
// subject would fill the Other section with noise.
func TestCommitsExcludesMerges(t *testing.T) {
	dir := newTestRepo(t)
	commitTo(t, dir, "feat: base")
	tagAt(t, dir, "v1.0.0")

	for _, args := range [][]string{{"checkout", "-q", "-b", "side"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	commitTo(t, dir, "fix: on the side branch")
	for _, args := range [][]string{
		{"checkout", "-q", "main"},
		{"merge", "-q", "--no-ff", "-m", "Merge branch 'side'", "side"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	tagAt(t, dir, "v1.1.0")

	commits, err := Commits(dir, "v1.0.0", "v1.1.0")
	if err != nil {
		t.Fatalf("Commits: %v", err)
	}
	for _, c := range commits {
		if c.Subject == "Merge branch 'side'" {
			t.Fatal("a merge commit reached the changelog")
		}
	}
	if len(commits) != 1 {
		t.Fatalf("got %d commits, want only the side-branch fix", len(commits))
	}
}
