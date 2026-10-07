package proc

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMergePATHKeepsCurrentAndAddsMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH augmentation is a macOS/Linux concern")
	}
	merged, added := mergePATH(
		"/opt/homebrew/bin:/usr/bin:/Users/x/.nvm/versions/node/v22/bin",
		"/usr/bin:/bin:/usr/sbin:/sbin",
		[]string{"/usr/local/bin", "/bin", "/opt/homebrew/bin/"},
	)
	want := "/opt/homebrew/bin:/usr/bin:/Users/x/.nvm/versions/node/v22/bin:/bin:/usr/sbin:/sbin:/usr/local/bin"
	if merged != want {
		t.Errorf("merged = %q\nwant     %q", merged, want)
	}
	wantAdded := []string{"/opt/homebrew/bin", "/Users/x/.nvm/versions/node/v22/bin", "/usr/local/bin"}
	if strings.Join(added, ":") != strings.Join(wantAdded, ":") {
		t.Errorf("added = %v, want %v", added, wantAdded)
	}
}

func TestMergePATHNoShellNoExtraIsIdentity(t *testing.T) {
	cur := strings.Join([]string{"a", "b"}, string(os.PathListSeparator))
	merged, added := mergePATH("", cur, nil)
	if merged != cur || len(added) != 0 {
		t.Errorf("mergePATH = %q, %v; want %q, none", merged, added, cur)
	}
}

func TestWellKnownBinDirs(t *testing.T) {
	darwin := wellKnownBinDirs("darwin", "/Users/x")
	if darwin[0] != "/opt/homebrew/bin" {
		t.Errorf("darwin dirs must lead with Homebrew, got %v", darwin)
	}
	if !contains(darwin, filepath.Join("/Users/x", ".local", "bin")) {
		t.Errorf("darwin dirs miss ~/.local/bin: %v", darwin)
	}
	if contains(wellKnownBinDirs("linux", ""), filepath.Join("", ".local", "bin")) {
		t.Error("per-user dirs must be skipped without a home dir")
	}
}

// The real login shell answers with a PATH that contains /usr/bin on every
// macOS/Linux host; banner noise from rc files must not leak into it.
func TestLoginShellPATH(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getenv("SHELL") == "" {
		t.Skip("needs a POSIX login shell")
	}
	got := loginShellPATH(10 * time.Second)
	if got == "" {
		t.Skip("login shell did not answer (restricted CI shell)")
	}
	if strings.Contains(got, "\n") || strings.Contains(got, pathMarker) {
		t.Fatalf("PATH carries shell noise: %q", got)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
