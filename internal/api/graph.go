package api

import (
	"net/http"
	"strconv"

	"github.com/bilal/swarmgo/internal/orchestration"
)

// graphNode is one entity in the workspace collaboration network. ID is
// type-prefixed ("agent:<id>", "task:<id>", "flow:<id>", "skill:<slug>",
// "mcp:<id>") so ids are unique across types and edges can reference them.
type graphNode struct {
	ID     string `json:"id"`
	Type   string `json:"type"` // agent | task | flow | skill | mcp
	Label  string `json:"label"`
	Sub    string `json:"sub,omitempty"`    // subtitle (provider/model, status, …)
	Color  string `json:"color,omitempty"`  // agent accent (drives the cluster hue)
	Emoji  string `json:"emoji,omitempty"`  // agent avatar glyph, if any
	Group  string `json:"group,omitempty"`  // owning hub id (for cluster layout)
	Status string `json:"status,omitempty"` // task board state
	Desc   string `json:"desc,omitempty"`   // longer description (task tooltip)

	// Live activity (agents only): whether the agent has an in-flight run right
	// now, what kind (task|flow|chat|schedule|heartbeat) and the type-prefixed id
	// of the task/flow it is running (empty for chat/schedule/heartbeat).
	Running   bool   `json:"running,omitempty"`
	RunKind   string `json:"runKind,omitempty"`
	RunTarget string `json:"runTarget,omitempty"`
}

// graphEdge links two graph nodes. Kind names the relationship so the frontend
// can color/label it: owns (agent→task), created (agent→task), runs (task→flow),
// uses (flow→agent), skill (agent→skill), mcp (agent→server).
type graphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Kind   string `json:"kind"`
}

type workspaceGraph struct {
	Nodes []graphNode    `json:"nodes"`
	Edges []graphEdge    `json:"edges"`
	Stats map[string]int `json:"stats"`
}

// registerGraphRoutes registers the relationship-graph endpoints: the
// workspace-wide collaboration network and the per-agent memory knowledge graph.
func (s *Server) registerGraphRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/graph", s.handleWorkspaceGraph)
	mux.HandleFunc("GET /api/agents/{id}/memory-graph", s.handleMemoryGraph)
}

// handleWorkspaceGraph returns the workspace collaboration network: agents,
// tasks and flows as nodes, with edges for ownership/authorship (agent↔task),
// flow-backing (task→flow) and flow membership (flow→agent). This is the data
// behind the "Ağ" visualization.
func (s *Server) handleWorkspaceGraph(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	ctx := r.Context()

	agents, err := wsp.DB.ListAgents(ctx)
	if writeDBError(w, err, "") {
		return
	}
	tasks, err := wsp.DB.ListTasks(ctx)
	if writeDBError(w, err, "") {
		return
	}
	flows, err := wsp.DB.ListFlows(ctx)
	if writeDBError(w, err, "") {
		return
	}
	mcpServers, err := wsp.DB.ListMCPServers(ctx)
	if writeDBError(w, err, "") {
		return
	}

	const (
		agentPfx = "agent:"
		taskPfx  = "task:"
		flowPfx  = "flow:"
		skillPfx = "skill:"
		mcpPfx   = "mcp:"
	)

	nodes := make([]graphNode, 0, len(agents)+len(tasks)+len(flows))
	edges := make([]graphEdge, 0)

	// Per-agent live activity: which agent currently has an in-flight streaming
	// run, derived from the process-wide running session set joined to this
	// workspace's sessions. A task/flow-kind running session bonds the agent to
	// that task/flow node in the live view; chat/schedule/heartbeat just mark it
	// as running (a glow, no specific target).
	type activity struct{ kind, target string }
	agentAct := map[string]activity{}
	running := map[string]bool{}
	for _, id := range s.runs.activeSessionIDs() {
		running[id] = true // chat-streaming turns
	}
	for _, id := range wsp.Runtime.ActiveSessionIDs() {
		running[id] = true // autonomous + flow/task runs (schedule/heartbeat/spawn/flow)
	}
	if len(running) > 0 {
		if sessions, serr := wsp.DB.ListSessions(ctx, ""); serr == nil {
			for _, sess := range sessions {
				if !running[sess.ID] || sess.AgentID == "" {
					continue
				}
				target := ""
				switch sess.Kind {
				case "task":
					if sess.SourceID != "" {
						target = taskPfx + sess.SourceID
					}
				case "flow":
					if sess.SourceID != "" {
						target = flowPfx + sess.SourceID
					}
				}
				agentAct[sess.AgentID] = activity{kind: sess.Kind, target: target}
			}
		}
	}

	agentExists := make(map[string]bool, len(agents))
	for _, a := range agents {
		agentExists[a.ID] = true
		sub := a.Provider
		if a.Model != "" {
			sub = a.Provider + " · " + a.Model
		}
		node := graphNode{
			ID:    agentPfx + a.ID,
			Type:  "agent",
			Label: a.Name,
			Sub:   sub,
			Color: a.Color,
			Emoji: a.Avatar,
		}
		if act, ok := agentAct[a.ID]; ok {
			node.Running = true
			node.RunKind = act.kind
			node.RunTarget = act.target
		}
		nodes = append(nodes, node)
	}

	for _, t := range tasks {
		label := t.Title
		if label == "" {
			label = "(başlıksız görev)"
		}
		// Cluster a task under its owner agent when one exists, so the layout
		// draws tasks as spokes around their agent hub (like the reference graph).
		group := ""
		if t.OwnerAgentID != "" && agentExists[t.OwnerAgentID] {
			group = agentPfx + t.OwnerAgentID
		}
		nodes = append(nodes, graphNode{
			ID:     taskPfx + t.ID,
			Type:   "task",
			Label:  label,
			Status: t.BoardState,
			Group:  group,
			Desc:   t.Description,
		})
		if t.OwnerAgentID != "" && agentExists[t.OwnerAgentID] {
			edges = append(edges, graphEdge{Source: agentPfx + t.OwnerAgentID, Target: taskPfx + t.ID, Kind: "owns"})
		}
		if t.CreatedBy != "" && t.CreatedBy != t.OwnerAgentID && agentExists[t.CreatedBy] {
			edges = append(edges, graphEdge{Source: agentPfx + t.CreatedBy, Target: taskPfx + t.ID, Kind: "created"})
		}
		if t.FlowID != "" {
			edges = append(edges, graphEdge{Source: taskPfx + t.ID, Target: flowPfx + t.FlowID, Kind: "runs"})
		}
	}

	for _, f := range flows {
		nodes = append(nodes, graphNode{
			ID:    flowPfx + f.ID,
			Type:  "flow",
			Label: f.Name,
			Sub:   f.Description,
		})
		// A flow "uses" every distinct agent referenced by its agent nodes — this
		// is the multi-agent collaboration signal (two agents wired into one flow).
		seen := map[string]bool{}
		if g, perr := orchestration.ParseGraph(f.Graph); perr == nil {
			for _, n := range g.Nodes {
				if n.Type == orchestration.NodeAgent && n.AgentID != "" && !seen[n.AgentID] && agentExists[n.AgentID] {
					seen[n.AgentID] = true
					edges = append(edges, graphEdge{Source: flowPfx + f.ID, Target: agentPfx + n.AgentID, Kind: "uses"})
				}
			}
		}
	}

	// Skills: a shared library node per distinct slug used by any agent, with an
	// agent→skill edge. Reveals which agents share which capabilities.
	skillSeen := map[string]bool{}
	skillCount := 0
	for _, a := range agents {
		for _, slug := range a.Skills {
			if slug == "" {
				continue
			}
			if !skillSeen[slug] {
				skillSeen[slug] = true
				skillCount++
				nodes = append(nodes, graphNode{ID: skillPfx + slug, Type: "skill", Label: slug})
			}
			edges = append(edges, graphEdge{Source: agentPfx + a.ID, Target: skillPfx + slug, Kind: "skill"})
		}
	}

	// MCP servers: one node per enabled server; an agent→server edge for every
	// MCP-enabled agent (coarse access signal — SwarmGo gates tools per agent via
	// an allowlist, not per server, so this shows "which agents can reach MCP").
	mcpCount := 0
	for _, srv := range mcpServers {
		if !srv.Enabled {
			continue
		}
		mcpCount++
		nodes = append(nodes, graphNode{ID: mcpPfx + srv.ID, Type: "mcp", Label: srv.Name, Sub: srv.Transport})
		for _, a := range agents {
			if a.MCPEnabled {
				edges = append(edges, graphEdge{Source: agentPfx + a.ID, Target: mcpPfx + srv.ID, Kind: "mcp"})
			}
		}
	}

	writeJSON(w, http.StatusOK, workspaceGraph{
		Nodes: nodes,
		Edges: edges,
		Stats: map[string]int{
			"agents": len(agents),
			"tasks":  len(tasks),
			"flows":  len(flows),
			"skills": skillCount,
			"mcp":    mcpCount,
			"edges":  len(edges),
		},
	})
}

// handleMemoryGraph returns an agent's memory knowledge graph: each memory is a
// node and pairs with lexical-cosine similarity at/above ?threshold (default
// 0.18) are linked, capped at ?max edges (default 400).
func (s *Server) handleMemoryGraph(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")

	threshold := 0.18
	if v := r.URL.Query().Get("threshold"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			threshold = f
		}
	}
	maxEdges := 400
	if v := r.URL.Query().Get("max"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxEdges = n
		}
	}

	if _, err := ws(r).DB.GetAgent(r.Context(), agentID); err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	// Store.Graph already returns non-nil node/edge slices, so the client always
	// receives arrays.
	graph, err := ws(r).Runtime.Memory().Graph(r.Context(), agentID, threshold, maxEdges)
	if writeDBError(w, err, "") {
		return
	}
	writeJSON(w, http.StatusOK, graph)
}
