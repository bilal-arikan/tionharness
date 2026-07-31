package exttools

import "testing"

// Version banners are wildly inconsistent across the catalog — some prefix the
// tool name, some say "version", ffmpeg buries it in a build string. The parser
// must pull the same normalised triple out of all of them.
func TestParseVersion(t *testing.T) {
	cases := []struct {
		in                          string
		wantOK                      bool
		wantMaj, wantMin, wantPatch int
	}{
		{"rtk 0.9.0", true, 0, 9, 0},
		{"sqz v1.3.0", true, 1, 3, 0},
		{"ffmpeg version 7.1-full_build-www.gyan.dev", true, 7, 1, 0},
		{"codebase-memory-mcp 0.8.1", true, 0, 8, 1},
		{"10.4.2", true, 10, 4, 2},
		{"", false, 0, 0, 0},
		{"no version here", false, 0, 0, 0},
		// A bare integer is NOT a version: treating "usage: tool [-3]" as 3.0.0
		// would fabricate a comparison out of noise.
		{"usage: whisper-cli [options]", false, 0, 0, 0},
	}
	for _, c := range cases {
		maj, min, patch, ok := ParseVersion(c.in)
		if ok != c.wantOK {
			t.Fatalf("ParseVersion(%q) ok = %v, want %v", c.in, ok, c.wantOK)
		}
		if !ok {
			continue
		}
		if maj != c.wantMaj || min != c.wantMin || patch != c.wantPatch {
			t.Fatalf("ParseVersion(%q) = %d.%d.%d, want %d.%d.%d",
				c.in, maj, min, patch, c.wantMaj, c.wantMin, c.wantPatch)
		}
	}
}

func TestLocalVersionRejectsMissingProbe(t *testing.T) {
	if _, err := LocalVersion(t.Context(), "", []string{"--version"}); err == nil {
		t.Fatal("empty path must error, not silently report no version")
	}
	if _, err := LocalVersion(t.Context(), "some-tool", nil); err == nil {
		t.Fatal("missing version args must error")
	}
}

// Repo is derived from the catalog URL rather than stored separately, so the two
// can never drift. Non-GitHub tools (ffmpeg) must yield "" — no release feed.
func TestRepoDerivation(t *testing.T) {
	cases := map[string]string{
		"rtk":                 "rtk-ai/rtk",
		"sqz":                 "ojuschugh1/sqz",
		"mmdc":                "mermaid-js/mermaid-cli",
		"codebase-memory-mcp": "DeusData/codebase-memory-mcp",
		"piper":               "OHF-Voice/piper1-gpl",
		"whisper-cli":         "ggml-org/whisper.cpp",
		"ffmpeg":              "",
	}
	for name, want := range cases {
		tool := Find(name)
		if tool == nil {
			t.Fatalf("catalog is missing %q", name)
		}
		if got := tool.Repo(); got != want {
			t.Fatalf("%s Repo() = %q, want %q", name, got, want)
		}
	}
}

// Every catalog entry must declare an update path, and a "command" entry must
// actually carry a command — otherwise the UI would offer a button that 409s.
func TestCatalogUpdateSpecsAreComplete(t *testing.T) {
	for _, tool := range Catalog {
		switch tool.Update.Kind {
		case UpdateCommand:
			if tool.Update.Command == "" {
				t.Fatalf("%s: command update with no command", tool.Name)
			}
			if tool.Update.UpdateCommandLine() == "" {
				t.Fatalf("%s: command line renders empty", tool.Name)
			}
		case UpdateManual:
			if tool.Update.Note == "" {
				t.Fatalf("%s: manual update with no instructions — the UI would show a blank 409", tool.Name)
			}
		default:
			t.Fatalf("%s: unknown update kind %q", tool.Name, tool.Update.Kind)
		}
	}
}
