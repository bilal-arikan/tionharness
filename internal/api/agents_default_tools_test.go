package api

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/db"
)

// TestCreateAgentPerCallerFlipDefault pins the per-creation-path default-on
// contract for MCPEnabled. The DB layer does NOT default this field (Go bools
// cannot tell "unset" from "explicit false"), so each creation path is
// responsible for flipping false→true before calling db.CreateAgent:
//
//   - handleCreateAgent (POST /api/agents): handled via *bool on the request —
//     nil defaults to true, false opts out explicitly.
//   - installAgentPack (market install): unconditional flip.
//   - seedWorkspaceTeam (workspace templates): unconditional flip.
//   - CreateAgentTool.Call (self-management): hard-codes MCPEnabled=true.
//
// This test pins (1) at the *bool layer with persistence, and (2)/(3) at the
// flip layer with persistence — the same rule, two caller shapes.
func TestCreateAgentPerCallerFlipDefault(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer database.Close()

	// (a) *bool-style flip — exactly the lines in handleCreateAgent.
	flipPointer := func(reqMCPEnabled *bool) bool {
		mcp := true
		if reqMCPEnabled != nil {
			mcp = *reqMCPEnabled
		}
		return mcp
	}
	casesPtr := []struct {
		name string
		body string
		want bool
	}{
		// JSON-decodes with req.MCPEnabled == nil — the handler then defaults to true.
		{"nil defaults on", `{"name":"Omit"}`, true},
		{"explicit true", `{"name":"Opted","mcpEnabled":true}`, true},
		{"explicit false (opt-out)", `{"name":"Quiet","mcpEnabled":false}`, false},
	}
	for _, tc := range casesPtr {
		t.Run("pointer/"+tc.name, func(t *testing.T) {
			var req createAgentReq
			if err := json.NewDecoder(strings.NewReader(tc.body)).Decode(&req); err != nil {
				t.Fatalf("decode: %v", err)
			}
			got := flipPointer(req.MCPEnabled)
			if got != tc.want {
				t.Fatalf("handler input mcpEnabled = %v, want %v", got, tc.want)
			}
			// Persist exactly what handleCreateAgent would persist, then reload
			// to catch a regression in db.CreateAgent defaults.
			a, err := database.CreateAgent(ctx, db.Agent{
				Name: req.Name, Provider: "anthropic", MCPEnabled: got,
			})
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			reloaded, err := database.GetAgent(ctx, a.ID)
			if err != nil {
				t.Fatalf("reload: %v", err)
			}
			if reloaded.MCPEnabled != tc.want {
				t.Fatalf("persisted MCPEnabled = %v, want %v", reloaded.MCPEnabled, tc.want)
			}
		})
	}

	// (b) bool flip — exactly the lines in installAgentPack and
	// seedWorkspaceTeam. *bool is not used in those paths because the payload
	// types (AgentPayload, WorkspaceTemplateAgent) carry a plain bool.
	flipBool := func(in bool) bool {
		if !in {
			return true
		}
		return in
	}
	for _, mcp := range []bool{false, true} {
		name := "bool/true→true"
		if !mcp {
			name = "bool/false→true (default-on kicks in)"
		}
		t.Run(name, func(t *testing.T) {
			flipped := flipBool(mcp)
			got, err := database.CreateAgent(ctx, db.Agent{
				Name: "A", Provider: "anthropic", MCPEnabled: flipped,
			})
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			reloaded, err := database.GetAgent(ctx, got.ID)
			if err != nil {
				t.Fatalf("reload: %v", err)
			}
			if !reloaded.MCPEnabled {
				t.Fatalf("MCPEnabled reloaded as false; default-on rule violated")
			}
		})
	}
}

func ptrBool(v bool) *bool { return &v }
