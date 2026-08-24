package exttools

import (
	"strings"
	"testing"
)

// TionHarness ships to Linux servers as well as Windows desktops. These tests pin
// the platform-dependent halves of the catalog so a Windows-only assumption
// cannot silently reach a Ubuntu box, where the tool that would run it does not
// exist. They deliberately call the *goos-parameterised* constructors rather
// than reading the built Catalog, so BOTH branches are covered from either host.

func TestWingetSpecIsWindowsOnly(t *testing.T) {
	const note = "apt ile güncelle"

	win := wingetSpec("windows", "Git.Git", note)
	if win.Kind != UpdateCommand || win.Command != "winget" {
		t.Fatalf("windows: winget komutu bekleniyordu, alınan %+v", win)
	}

	for _, goos := range []string{"linux", "darwin"} {
		got := wingetSpec(goos, "Git.Git", note)
		if got.Kind != UpdateManual {
			t.Errorf("%s: manual bekleniyordu, alınan %q", goos, got.Kind)
		}
		if got.Command != "" {
			t.Errorf("%s: komut çalıştırılmamalı, alınan %q", goos, got.Command)
		}
		if got.Note != note {
			t.Errorf("%s: not aktarılmadı: %q", goos, got.Note)
		}
		// The clipboard chip renders from UpdateCommandLine(); it must not hand a
		// Linux user a winget line.
		if line := got.UpdateCommandLine(); line != "" {
			t.Errorf("%s: kopyalanacak komut boş olmalı, alınan %q", goos, line)
		}
	}
}

func TestBunUpdateSpecPerPlatform(t *testing.T) {
	win := bunUpdateSpec("windows")
	if win.Command != "winget" {
		t.Errorf("windows: winget bekleniyordu, alınan %q", win.Command)
	}
	for _, goos := range []string{"linux", "darwin"} {
		got := bunUpdateSpec(goos)
		if got.Kind != UpdateCommand || got.Command != "bun" {
			t.Errorf("%s: `bun upgrade` bekleniyordu, alınan %+v", goos, got)
		}
		if got.UpdateCommandLine() != "bun upgrade" {
			t.Errorf("%s: komut satırı %q", goos, got.UpdateCommandLine())
		}
	}
}

// node/python stay manual everywhere — only the instruction text is localised to
// the platform's actual package manager. A note naming the WRONG tool is the bug
// being guarded against here.
func TestManualNotesNameTheRightToolPerPlatform(t *testing.T) {
	cases := []struct {
		goos string
		// forbidden: tools that do not exist on this platform
		forbidden []string
		// wanted: at least one of these must appear
		wanted []string
	}{
		{"windows", []string{"apt", "brew", "deadsnakes", "NodeSource"}, []string{"winget", "nodejs.org", "python.org"}},
		{"linux", []string{"winget", "brew"}, []string{"apt", "NodeSource", "deadsnakes", "pyenv"}},
		{"darwin", []string{"winget", "apt", "deadsnakes"}, []string{"brew"}},
	}
	for _, c := range cases {
		for name, spec := range map[string]UpdateSpec{
			"node":   nodeUpdateSpec(c.goos),
			"python": pythonUpdateSpec(c.goos),
		} {
			if spec.Kind != UpdateManual {
				t.Errorf("%s/%s: her platformda manual kalmalı, alınan %q", c.goos, name, spec.Kind)
			}
			if spec.Note == "" {
				t.Errorf("%s/%s: not boş", c.goos, name)
			}
			for _, bad := range c.forbidden {
				if strings.Contains(spec.Note, bad) {
					t.Errorf("%s/%s: notta bu platformda olmayan %q geçiyor", c.goos, name, bad)
				}
			}
			hit := false
			for _, w := range c.wanted {
				if strings.Contains(spec.Note, w) {
					hit = true
					break
				}
			}
			if !hit {
				t.Errorf("%s/%s: not platforma uygun bir yol göstermiyor (beklenen biri: %v)", c.goos, name, c.wanted)
			}
		}
	}
}

// The Ubuntu foot-gun must be called out explicitly: replacing the system
// interpreter in place is a known way to break apt's own tooling.
func TestPythonLinuxNoteWarnsAboutSystemInterpreter(t *testing.T) {
	note := pythonUpdateSpec("linux").Note
	for _, want := range []string{"apt", "pyenv"} {
		if !strings.Contains(note, want) {
			t.Errorf("linux python notunda %q geçmeli: %q", want, note)
		}
	}
}

// Nothing in the live catalog may point a non-Windows host at winget. On a
// Windows host this passes trivially; on a Linux CI host it is the real guard.
func TestCatalogHasNoWingetOffWindows(t *testing.T) {
	for _, tool := range Catalog {
		line := tool.Update.UpdateCommandLine()
		if strings.Contains(line, "winget") && !isWindowsHost() {
			t.Errorf("%s: bu platformda winget komutu sunuluyor: %q", tool.Name, line)
		}
	}
}

func isWindowsHost() bool { return gitProjectURL == "https://github.com/git-for-windows/git" }
