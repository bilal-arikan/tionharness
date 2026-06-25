package skills

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLiveGitHubImport bulk-imports a real collection into a temp workspace and
// verifies skills + nested bundled resources land on disk. Network-gated.
func TestLiveGitHubImport(t *testing.T) {
	if os.Getenv("SWARMGO_LIVE_TEST") != "1" {
		t.Skip("set SWARMGO_LIVE_TEST=1 to run the live GitHub import")
	}
	ws := t.TempDir()
	s := New("", ws)
	res, err := s.ImportCollection("github", "coreyhaines31/marketingskills", nil, "", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("imported=%d skipped=%d warnings=%v", len(res.Imported), len(res.Skipped), res.Warnings)
	if len(res.Imported) < 40 {
		t.Errorf("expected ~45 skills imported, got %d", len(res.Imported))
	}
	// Spot-check one skill with nested resources (ab-testing has references/ + evals/).
	abDir := filepath.Join(ws, "ab-testing")
	if _, err := os.Stat(filepath.Join(abDir, "SKILL.md")); err != nil {
		t.Errorf("ab-testing/SKILL.md not written: %v", err)
	}
	withNested := 0
	for _, ir := range res.Imported {
		for _, f := range ir.Files {
			if filepath.ToSlash(f) != f || len(f) > 0 && (f[0] != '/') {
				if containsSlash(f) {
					withNested++
				}
			}
		}
	}
	t.Logf("bundled files with nested paths across import: %d", withNested)
	if withNested == 0 {
		t.Errorf("expected at least one nested bundled file to be preserved")
	}
}

func containsSlash(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return true
		}
	}
	return false
}

// TestLiveGitHubScan is a network-gated smoke test against the real community skill
// collections. Run with: SWARMGO_LIVE_TEST=1 go test -run TestLiveGitHubScan ./internal/skills/
func TestLiveGitHubScan(t *testing.T) {
	if os.Getenv("SWARMGO_LIVE_TEST") != "1" {
		t.Skip("set SWARMGO_LIVE_TEST=1 to run the live GitHub scan")
	}
	repos := []string{
		"juliusbrussee/caveman",
		"leonxlnx/taste-skill",
		"coreyhaines31/marketingskills",
	}
	s := New("", t.TempDir())
	for _, repo := range repos {
		res, err := s.ScanCollection("github", repo)
		if err != nil {
			t.Errorf("%s: scan failed: %v", repo, err)
			continue
		}
		if len(res.Skills) == 0 {
			t.Errorf("%s: discovered 0 skills", repo)
			continue
		}
		nested := 0
		for _, sk := range res.Skills {
			if len(sk.Files) > 0 {
				nested++
			}
		}
		t.Logf("%s: %d skills (%d with bundled files), warnings=%v", repo, len(res.Skills), nested, res.Warnings)
		// Spot-check the first few names came through.
		for i, sk := range res.Skills {
			if i >= 3 {
				break
			}
			t.Logf("    - %s | %s | %s", sk.Slug, sk.Name, sk.Description)
		}
	}
}
