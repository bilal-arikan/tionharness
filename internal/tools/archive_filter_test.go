package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// listIDs runs a paginated list tool and returns the ids on its first page.
func listIDs(t *testing.T, tool Tool, input string) map[string]bool {
	t.Helper()
	out, err := tool.Call(context.Background(), json.RawMessage(input))
	if err != nil {
		t.Fatalf("%s(%s): %v", tool.Def().Name, input, err)
	}
	var page struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &page); err != nil {
		t.Fatalf("decode %s: %v (%s)", tool.Def().Name, err, out)
	}
	ids := map[string]bool{}
	for _, it := range page.Items {
		ids[it.ID] = true
	}
	return ids
}

// list_agents / list_artifacts / list_automations follow the list_tasks
// convention: live items by default, archived=true returns only archived ones.
func TestListToolsArchiveFilter(t *testing.T) {
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()

	liveAgent, _ := d.CreateAgent(ctx, db.Agent{Name: "live"})
	shelvedAgent, _ := d.CreateAgent(ctx, db.Agent{Name: "shelved"})
	if _, err := d.SetAgentArchived(ctx, shelvedAgent.ID, true); err != nil {
		t.Fatal(err)
	}
	liveArt, _ := d.CreateArtifact(ctx, db.Artifact{Title: "live", Kind: "markdown", Content: "x"})
	shelvedArt, _ := d.CreateArtifact(ctx, db.Artifact{Title: "shelved", Kind: "markdown", Content: "x"})
	if _, err := d.SetArtifactArchived(ctx, shelvedArt.ID, true); err != nil {
		t.Fatal(err)
	}
	mk := func(name string) db.Automation {
		a, err := d.CreateAutomation(ctx, db.Automation{Name: name, TriggerKind: db.TriggerTag, TriggerTag: "t", TargetAgentID: liveAgent.ID, PromptTemplate: "go", MaxIterations: 3})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	liveAuto, shelvedAuto := mk("live"), mk("shelved")
	if err := d.SetAutomationArchived(ctx, shelvedAuto.ID, true); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		tool          Tool
		live, shelved string
	}{
		{NewListAgentsTool(d, ""), liveAgent.ID, shelvedAgent.ID},
		{NewListArtifactsTool(d, ""), liveArt.ID, shelvedArt.ID},
		{NewListAutomationsTool(d, ""), liveAuto.ID, shelvedAuto.ID},
	} {
		def := listIDs(t, c.tool, `{}`)
		if !def[c.live] || def[c.shelved] {
			t.Errorf("%s default = %v, want only the live item", c.tool.Def().Name, def)
		}
		arch := listIDs(t, c.tool, `{"archived":true}`)
		if arch[c.live] || !arch[c.shelved] {
			t.Errorf("%s archived=true = %v, want only the archived item", c.tool.Def().Name, arch)
		}
	}
}
