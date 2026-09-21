package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveInstructionFile covers the full candidate matrix. CLAUDE.md wins
// when both are present; AGENTS.md is the fallback; an empty directory resolves
// to "" so the caller can omit the prompt line entirely.
//
// Case sensitivity is the FILESYSTEM's, not this function's: on Windows and on
// macOS's default case-insensitive APFS a `claude.md` on disk satisfies the
// `CLAUDE.md` stat, while on Linux it does not. These cases deliberately use the
// canonical casing only, so the test asserts the same thing on every platform.
func TestResolveInstructionFile(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  string
	}{
		{name: "both present, CLAUDE.md wins", files: []string{"CLAUDE.md", "AGENTS.md"}, want: "CLAUDE.md"},
		{name: "only CLAUDE.md", files: []string{"CLAUDE.md"}, want: "CLAUDE.md"},
		{name: "only AGENTS.md", files: []string{"AGENTS.md"}, want: "AGENTS.md"},
		{name: "neither", files: nil, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, f), []byte("# rules\n"), 0o600); err != nil {
					t.Fatalf("write %s: %v", f, err)
				}
			}

			got, err := ResolveInstructionFile(dir)
			if err != nil {
				t.Fatalf("ResolveInstructionFile: %v", err)
			}
			if got != tc.want {
				t.Fatalf("resolved %q, want %q", got, tc.want)
			}

			line := InstructionFilePromptLine(dir)
			if tc.want == "" {
				if line != "" {
					t.Fatalf("expected no pointer line for a dir with no instruction file, got %q", line)
				}
				return
			}
			// The emitted line must name the file that was actually found, and must
			// not mention the candidate that lost (or was absent).
			if !strings.Contains(line, "`"+tc.want+"`") {
				t.Fatalf("pointer line %q does not name %q", line, tc.want)
			}
			for _, other := range InstructionFileCandidates {
				if other != tc.want && strings.Contains(line, other) {
					t.Fatalf("pointer line %q mentions %q, which is not the resolved file", line, other)
				}
			}
		})
	}
}

// An empty dir argument is "no working directory", not an error: the callers
// already skip the whole block in that case, and probing "" must not stat the
// process's cwd by accident.
func TestResolveInstructionFileEmptyDir(t *testing.T) {
	got, err := ResolveInstructionFile("  ")
	if err != nil {
		t.Fatalf("ResolveInstructionFile(blank): %v", err)
	}
	if got != "" {
		t.Fatalf("resolved %q for a blank dir, want \"\"", got)
	}
}

// A stat failure that is NOT "does not exist" must be returned, never reported
// as "this directory has no instruction file" — the two mean different things,
// and collapsing them would present an unreadable working directory as a repo
// without conventions. The failure is injected because it cannot be provoked
// portably: Windows reports an ENOTDIR-style path failure as ErrNotExist.
func TestResolveInstructionFileStatErrorSurfaces(t *testing.T) {
	boom := errors.New("permission denied")
	orig := statInstructionFile
	statInstructionFile = func(string) (os.FileInfo, error) { return nil, boom }
	t.Cleanup(func() { statInstructionFile = orig })

	_, err := ResolveInstructionFile(t.TempDir())
	if !errors.Is(err, boom) {
		t.Fatalf("expected the stat error to be returned, got %v", err)
	}

	// The prompt-facing wrapper has no error channel, so it must state the
	// failure in the line rather than silently emitting nothing.
	line := InstructionFilePromptLine(t.TempDir())
	if !strings.Contains(line, "Could not probe") {
		t.Fatalf("expected the prompt line to report the probe failure, got %q", line)
	}
}
