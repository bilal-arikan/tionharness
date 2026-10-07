package exttools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/procwatch"
)

// The codex update command must follow whoever owns the install: running the
// wrong package manager leaves the binary TionHarness executes untouched.

func TestCodexUpdateSpecNpmSymlinkLayout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX npm layout uses symlinks")
	}
	prefix := t.TempDir()
	target := filepath.Join(prefix, "lib", "node_modules", "@openai", "codex", "bin", "codex.js")
	mustWrite(t, target)
	link := filepath.Join(prefix, "bin", "codex")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	spec, ok := codexUpdateSpec(link)
	if !ok || spec.Kind != UpdateCommand || spec.Command != "npm" {
		t.Fatalf("npm kurulumu tanınmadı: %+v ok=%v", spec, ok)
	}
	wantPrefix, _ := filepath.EvalSymlinks(prefix)
	want := []string{"install", "-g", "--prefix", wantPrefix, "@openai/codex@latest"}
	if len(spec.Args) != len(want) {
		t.Fatalf("args = %q, want %q", spec.Args, want)
	}
	for i := range want {
		if spec.Args[i] != want[i] {
			t.Fatalf("args = %q, want %q", spec.Args, want)
		}
	}
}

func TestCodexUpdateSpecNpmShimLayout(t *testing.T) {
	// Windows: codex.cmd is a plain file next to node_modules\@openai\codex.
	prefix := t.TempDir()
	if err := os.MkdirAll(filepath.Join(prefix, "node_modules", "@openai", "codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(prefix, "codex.cmd")
	mustWrite(t, shim)

	spec, ok := codexUpdateSpec(shim)
	if !ok || spec.Command != "npm" || spec.Args[3] != prefix {
		t.Fatalf("npm shim kurulumu tanınmadı: %+v ok=%v", spec, ok)
	}
}

func TestCodexUpdateSpecHomebrewCask(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "Caskroom", "codex", "0.160.1", "codex-aarch64-apple-darwin")
	mustWrite(t, bin)

	spec, ok := codexUpdateSpec(bin)
	if !ok || spec.UpdateCommandLine() != "brew upgrade --cask codex" {
		t.Fatalf("Homebrew cask tanınmadı: %+v ok=%v", spec, ok)
	}
}

func TestCodexUpdateSpecUnknownInstallFallsBackToManual(t *testing.T) {
	bin := filepath.Join(t.TempDir(), ".local", "bin", "codex")
	mustWrite(t, bin)

	if _, ok := codexUpdateSpec(bin); ok {
		t.Fatal("sahibi bilinmeyen kurulum için komut önerilmemeli")
	}
	tool := Find(CodexToolName)
	if got := tool.EffectiveUpdate(bin); got.Kind != UpdateManual || got.Note == "" {
		t.Fatalf("manuel nota düşmeli, alınan %+v", got)
	}
}

func TestUpdateCommandLineQuotesSpacedArgs(t *testing.T) {
	s := UpdateSpec{Kind: UpdateCommand, Command: "npm", Args: []string{"install", "-g", "--prefix", `C:\Users\Ad Soyad\npm`, "@openai/codex@latest"}}
	want := `npm install -g --prefix "C:\Users\Ad Soyad\npm" @openai/codex@latest`
	if got := s.UpdateCommandLine(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// A codex turn in flight must block the update: replacing the binary under it
// breaks that turn (and on Windows the running .exe is locked).
func TestRunUpdateRefusesWhileCodexRuns(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := procwatch.Default().Begin(ctx, procwatch.Meta{Kind: procwatch.KindProvider, Label: "codex-cli", Command: "codex exec"})
	defer h.FinishCode(0)

	tool := *Find(CodexToolName)
	if RunningProcesses(tool) == 0 {
		t.Fatal("çalışan codex süreci sayılmadı")
	}
	tool.Update = UpdateSpec{Kind: UpdateCommand, Command: "npm", Args: []string{"--version"}}
	if _, err := RunUpdate(ctx, tool); !errors.Is(err, ErrToolBusy) {
		t.Fatalf("ErrToolBusy bekleniyordu, alınan %v", err)
	}
}

func mustWrite(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
}
