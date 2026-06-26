package market

import (
	"context"
	"os"
	"testing"
)

// TestLiveSkillsMPSearch hits the real skillsmp.com search API. Network-gated:
// SWARMGO_LIVE_TEST=1 go test -run TestLiveSkillsMPSearch ./internal/market/
func TestLiveSkillsMPSearch(t *testing.T) {
	if os.Getenv("SWARMGO_LIVE_TEST") != "1" {
		t.Skip("set SWARMGO_LIVE_TEST=1 to run the live skillsmp search")
	}
	entries, err := searchSkillsMP(context.Background(), "seo", 10)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("skillsmp 'seo': %d source-ref entries", len(entries))
	for i, e := range entries {
		if i >= 3 {
			break
		}
		t.Logf("  - %s | %s", e.Name, e.Source.URL)
	}
}

// TestLiveCrossAIToolsSearch fetches+caches the real (~12 MB) crossaitools listing
// and searches it locally. Network-gated.
func TestLiveCrossAIToolsSearch(t *testing.T) {
	if os.Getenv("SWARMGO_LIVE_TEST") != "1" {
		t.Skip("set SWARMGO_LIVE_TEST=1 to run the live crossaitools search")
	}
	s := New(t.TempDir(), t.TempDir())
	entries, err := s.searchCrossAITools(context.Background(), "commit", 10)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("crossaitools 'commit': %d source-ref entries", len(entries))
	if len(entries) == 0 {
		t.Fatal("expected matches for 'commit'")
	}
	for i, e := range entries {
		if i >= 3 {
			break
		}
		t.Logf("  - %s | %s", e.Name, e.Source.URL)
	}
}
