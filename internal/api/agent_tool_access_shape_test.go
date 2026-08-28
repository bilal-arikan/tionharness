package api

import (
	"encoding/json"
	"testing"
)

// The tool-access payload must carry the group cost rows under stable JSON keys —
// the composer's inspector and the agent tools screen both read them by name.
func TestAgentToolAccessRespCarriesGroupCost(t *testing.T) {
	raw, err := json.Marshal(agentToolAccessResp{
		AgentID: "A1",
		Groups: []agentToolGroup{{
			Key: "group:files", Kind: "builtin", Label: "files", Count: 2,
			Tools: []string{"Bash", "Read"}, FullTokens: 3100, CurrentTokens: 420,
		}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out struct {
		Groups []struct {
			Key           string `json:"key"`
			FullTokens    int    `json:"fullTokens"`
			CurrentTokens int    `json:"currentTokens"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Groups) != 1 {
		t.Fatalf("expected one group row, got %d (%s)", len(out.Groups), raw)
	}
	g := out.Groups[0]
	if g.Key != "group:files" {
		t.Fatalf("group key = %q", g.Key)
	}
	if g.CurrentTokens >= g.FullTokens {
		t.Fatalf("cost fields did not survive the round trip: %+v", g)
	}
}
