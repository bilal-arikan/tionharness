package ingest

import (
	"os"
	"testing"
)

// TestLiveGitHubScan is a network-gated smoke test against the real community skill
// collections. Run with: TIONHARNESS_LIVE_TEST=1 go test -run TestLiveGitHubScan ./internal/ingest/
func TestLiveGitHubScan(t *testing.T) {
	if os.Getenv("TIONHARNESS_LIVE_TEST") != "1" {
		t.Skip("set TIONHARNESS_LIVE_TEST=1 to run the live GitHub scan")
	}
	repos := []string{
		"juliusbrussee/caveman",
		"leonxlnx/taste-skill",
		"coreyhaines31/marketingskills",
	}
	for _, repo := range repos {
		sr, err := Scan("github", repo)
		if err != nil {
			t.Errorf("%s: scan failed: %v", repo, err)
			continue
		}
		byKind := map[string]int{}
		for _, it := range sr.Items {
			byKind[it.Kind]++
		}
		t.Logf("%s: %d items %v", repo, len(sr.Items), byKind)
		if len(sr.Items) == 0 {
			t.Errorf("%s: discovered 0 items", repo)
		}
	}
}
