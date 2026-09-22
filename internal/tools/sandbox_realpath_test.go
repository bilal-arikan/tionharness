package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// linkFixture builds root/ and a sibling outside/ holding secret.txt, returning
// both directories. The links under test are created by each case.
func linkFixture(t *testing.T) (root, outside string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "root")
	outside = filepath.Join(base, "outside")
	for _, d := range []string{root, outside} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("s"), 0o644); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	return root, outside
}

func makeJunction(t *testing.T, link, target string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("junctions are Windows-only")
	}
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Skipf("cannot create a junction here: %v: %s", err, out)
	}
}

func makeSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		// Needs Developer Mode or admin on Windows.
		t.Skipf("cannot create a symlink here: %v", err)
	}
}

func assertEscapeRejected(t *testing.T, sb Sandbox, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if got, err := sb.Resolve(p); err == nil {
			t.Errorf("Resolve(%q) = %q; want a sandbox-escape error", p, got)
		}
	}
}

// A junction created INSIDE Root that points outside passes the lexical check;
// the handle-based check must still reject it — for the junction itself, an
// existing file below it, and a not-yet-existing file below it (a write target).
func TestConfinedSandbox_RejectsJunctionEscapeInsideRoot(t *testing.T) {
	root, outside := linkFixture(t)
	makeJunction(t, filepath.Join(root, "j"), outside)
	sb := NewConfinedSandbox(root)
	assertEscapeRejected(t, sb,
		"j",
		filepath.Join("j", "secret.txt"),
		filepath.Join("j", "new", "file.txt"),
		filepath.Join(root, "j", "secret.txt"),
	)
}

func TestConfinedSandbox_RejectsSymlinkEscapeInsideRoot(t *testing.T) {
	root, outside := linkFixture(t)
	makeSymlink(t, outside, filepath.Join(root, "dirlink"))
	makeSymlink(t, filepath.Join(outside, "secret.txt"), filepath.Join(root, "filelink"))
	sb := NewConfinedSandbox(root)
	assertEscapeRejected(t, sb,
		"dirlink",
		filepath.Join("dirlink", "secret.txt"),
		filepath.Join("dirlink", "new.txt"),
		"filelink",
	)
}

// A dangling link must not be approved as a "does not exist yet" path: writing
// through it creates the file at the link's target, outside Root.
func TestConfinedSandbox_RejectsDanglingSymlink(t *testing.T) {
	root, outside := linkFixture(t)
	makeSymlink(t, filepath.Join(outside, "not-there.txt"), filepath.Join(root, "dangling"))
	assertEscapeRejected(t, NewConfinedSandbox(root), "dangling")
}

// Links that stay inside Root are legitimate and keep resolving to the lexical path.
func TestConfinedSandbox_AllowsLinksStayingInsideRoot(t *testing.T) {
	root, _ := linkFixture(t)
	inner := filepath.Join(root, "inner")
	if err := os.Mkdir(inner, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if runtime.GOOS == "windows" {
		makeJunction(t, filepath.Join(root, "j"), inner)
	} else {
		makeSymlink(t, inner, filepath.Join(root, "j"))
	}
	sb := NewConfinedSandbox(root)
	for _, p := range []string{"j", filepath.Join("j", "x.go"), filepath.Join("j", "a", "b.go")} {
		got, err := sb.Resolve(p)
		if err != nil {
			t.Errorf("in-root link path %q rejected: %v", p, err)
			continue
		}
		if want := filepath.Join(root, p); got != want {
			t.Errorf("Resolve(%q) = %q, want lexical %q", p, got, want)
		}
	}
}

// When Root itself is a junction, paths below it resolve to the junction target;
// both sides are resolved through handles, so they must still count as in-root.
func TestConfinedSandbox_JunctionRootStillAllowsItsOwnFiles(t *testing.T) {
	root, _ := linkFixture(t)
	link := filepath.Join(filepath.Dir(root), "rootlink")
	makeJunction(t, link, root)
	if err := os.WriteFile(filepath.Join(root, "x.go"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	sb := NewConfinedSandbox(link)
	for _, p := range []string{"", "x.go", "new.go"} {
		if _, err := sb.Resolve(p); err != nil {
			t.Errorf("Resolve(%q) under a junction root rejected: %v", p, err)
		}
	}
}
