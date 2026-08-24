package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/orchestration"
)

// defaultFlow describes a built-in flow shipped into every workspace store by
// EnsureDefaultFlows. Seed is the stable identity used both to skip re-seeding an
// existing default and to record it in the deletion ledger.
type defaultFlow struct {
	Seed  string
	Name  string
	Emoji string
	Tags  []string
	Graph orchestration.Graph
}

// defaultFlows is the single source of truth for the built-in flows every
// workspace gets. Agent nodes carry an empty AgentID here — EnsureDefaultFlows
// assigns the workspace's first agent (or leaves it empty when there is none
// yet, exactly like an instantiated gallery template the user must fill).
//
// Keep this graph in sync with the matching entry in the frontend template
// gallery (frontend/src/features/flows/flowTemplates.ts, id "default-starter").
var defaultFlows = []defaultFlow{
	{
		Seed:  "starter-answer-verify",
		Name:  "Yanıtla & Doğrula",
		Emoji: "✅",
		Tags:  []string{"varsayılan"},
		Graph: orchestration.Graph{
			Start:     "start",
			EdgeStyle: "smoothstep",
			Nodes: []orchestration.Node{
				{ID: "start", Type: orchestration.NodeStart, Title: "Başlangıç", Next: "answer", X: 100, Y: -60},
				{ID: "answer", Type: orchestration.NodeAgent, Title: "Yanıtla", Prompt: "Answer the user's request thoroughly and concretely:\n{{input}}", Next: "verify", X: 100, Y: 60},
				{ID: "verify", Type: orchestration.NodeAgent, Title: "Doğrula", Prompt: "Review the answer above for errors, gaps, or unsupported claims, then produce a corrected, final version:\n{{last}}", Next: "end", X: 100, Y: 220},
				{ID: "end", Type: orchestration.NodeEnd, Title: "Bitiş", X: 100, Y: 380},
			},
		},
	},
}

// seededFlowsLedgerName is a dotfile at the store root recording which default
// flow seed keys have ever been provisioned into this workspace. It is what makes
// a user deletion permanent: once a key is recorded, EnsureDefaultFlows never
// recreates that default, even though the flow row is gone.
const seededFlowsLedgerName = ".seeded-flows.json"

type seededFlowsLedger struct {
	Seeded []string `json:"seeded"`
}

// EnsureDefaultFlows provisions the built-in default flows into a workspace's
// store, idempotently and respecting user deletion:
//
//   - If a flow with the same Seed key already exists in the DB, it is left
//     untouched (already provisioned).
//   - Else if the deletion ledger records the key, the flow was seeded before and
//     the user deleted it → it is NOT resurrected.
//   - Else the flow is created (its agent nodes get the workspace's first agent,
//     or an empty agentId when there is none yet — CreateFlow does not validate,
//     so the flow still appears and the user assigns agents later), and the key
//     is recorded in the ledger.
//
// Existing workspaces are backfilled naturally: open() calls this on every
// startup, so a newly shipped default appears in every store on the next launch.
func EnsureDefaultFlows(ctx context.Context, database *db.DB, storeDir string) error {
	if database == nil || storeDir == "" {
		return nil
	}
	ledger := loadSeededFlowsLedger(storeDir)
	recorded := map[string]bool{}
	for _, s := range ledger.Seeded {
		recorded[s] = true
	}

	existing, err := database.ListFlows(ctx)
	if err != nil {
		return err
	}
	present := map[string]bool{}
	for _, f := range existing {
		if f.Seed != "" {
			present[f.Seed] = true
		}
	}

	agentID := firstAgentID(ctx, database)

	changed := false
	for _, df := range defaultFlows {
		if present[df.Seed] || recorded[df.Seed] {
			continue
		}

		// Copy the graph and assign the default agent to agent nodes so the flow is
		// runnable out of the box.
		graph := df.Graph
		nodes := make([]orchestration.Node, len(graph.Nodes))
		copy(nodes, graph.Nodes)
		for i := range nodes {
			if nodes[i].Type == orchestration.NodeAgent && nodes[i].AgentID == "" {
				nodes[i].AgentID = agentID
			}
		}
		graph.Nodes = nodes

		raw, err := json.Marshal(graph)
		if err != nil {
			return err
		}
		if _, err := database.CreateFlow(ctx, db.Flow{
			Name:  df.Name,
			Graph: string(raw),
			Emoji: df.Emoji,
			Tags:  df.Tags,
			Seed:  df.Seed,
		}); err != nil {
			return err
		}
		ledger.Seeded = append(ledger.Seeded, df.Seed)
		recorded[df.Seed] = true
		changed = true
	}

	if changed {
		return saveSeededFlowsLedger(storeDir, ledger)
	}
	return nil
}

// MigrateFlowsStartEnd upgrades every flow in the store to the start-node model:
// any graph lacking a start node gets one prepended (its Next = the old entry).
// Idempotent — run at workspace open so existing flows adopt the new required
// entry marker without manual editing.
func MigrateFlowsStartEnd(ctx context.Context, database *db.DB) error {
	if database == nil {
		return nil
	}
	flows, err := database.ListFlows(ctx)
	if err != nil {
		return err
	}
	for _, f := range flows {
		g, err := orchestration.ParseGraph(f.Graph)
		if err != nil {
			continue // unparseable graph — leave it for the user to fix
		}
		ng, changed := orchestration.MigrateAddStart(g)
		if !changed {
			continue
		}
		raw, err := json.Marshal(ng)
		if err != nil {
			continue
		}
		if err := database.UpdateFlow(ctx, db.Flow{ID: f.ID, Name: f.Name, Graph: string(raw)}); err != nil {
			return err
		}
	}
	return nil
}

// firstAgentID returns the newest agent's id (ListAgents is sorted newest-first)
// or "" when the workspace has no agents yet.
func firstAgentID(ctx context.Context, database *db.DB) string {
	agents, err := database.ListAgents(ctx)
	if err != nil || len(agents) == 0 {
		return ""
	}
	return agents[0].ID
}

func loadSeededFlowsLedger(storeDir string) seededFlowsLedger {
	var l seededFlowsLedger
	data, err := os.ReadFile(filepath.Join(storeDir, seededFlowsLedgerName))
	if err != nil {
		return l
	}
	_ = json.Unmarshal(data, &l)
	return l
}

func saveSeededFlowsLedger(storeDir string, l seededFlowsLedger) error {
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(storeDir, seededFlowsLedgerName), data, 0o644)
}
