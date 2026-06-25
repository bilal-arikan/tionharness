package market

import (
	"context"
	"os"
	"testing"
)

// TestLiveSkillsMP hits the real skillsmp.com API. Network-gated:
// SWARMGO_LIVE_TEST=1 go test -run TestLiveSkillsMP ./internal/market/
func TestLiveSkillsMP(t *testing.T) {
	if os.Getenv("SWARMGO_LIVE_TEST") != "1" {
		t.Skip("set SWARMGO_LIVE_TEST=1 to run the live skillsmp fetch")
	}
	entries, err := fetchSkillsMP(context.Background(), connectors["skillsmp"].info.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("skillsmp: %d source-ref entries", len(entries))
	if len(entries) == 0 {
		t.Fatal("expected at least one entry")
	}
	for i, e := range entries {
		if i >= 3 {
			break
		}
		t.Logf("  - %s | %s | %s", e.ID, e.Name, e.Source.URL)
	}
}
